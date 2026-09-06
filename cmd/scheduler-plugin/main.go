package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/scheduler"
)

type k8sPodList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			SchedulerName string `json:"schedulerName"`
			NodeName      string `json:"nodeName"`
		} `json:"spec"`
	} `json:"items"`
}

type k8sNodeList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

type k8sBinding struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Target struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Name       string `json:"name"`
	} `json:"target"`
}

func getInClusterHTTPClient() (*http.Client, string, string, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, "", "", fmt.Errorf("not running in a kubernetes cluster")
	}

	tokenBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, "", "", err
	}
	token := string(tokenBytes)

	caCert, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, "", "", err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
		Timeout: 10 * time.Second,
	}

	baseURL := fmt.Sprintf("https://%s:%s", host, port)
	return client, baseURL, token, nil
}

func runSchedulingReconciler(ctx context.Context, schedulerName string, plugin *scheduler.Plugin, logger *slog.Logger) {
	client, baseURL, token, err := getInClusterHTTPClient()
	if err != nil {
		logger.Warn("Scheduler running outside cluster or unable to read service account credentials", "err", err)
		return
	}

	logger.Info("Connected to Kubernetes API for predictive scheduling reconciler", "baseURL", baseURL)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 1. Query pending pods
			req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v1/pods?fieldSelector=status.phase=Pending", nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				logger.Warn("Failed to list pending pods", "err", err)
				continue
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var podList k8sPodList
			if err := json.Unmarshal(body, &podList); err != nil {
				continue
			}

			// Filter pods claimed by this scheduler
			for _, pod := range podList.Items {
				if pod.Spec.SchedulerName != schedulerName || pod.Spec.NodeName != "" {
					continue
				}

				logger.Info("Scheduling pending pod via predictive algorithm",
					"pod", pod.Metadata.Name,
					"namespace", pod.Metadata.Namespace)

				// 2. Query nodes
				nreq, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v1/nodes", nil)
				if err != nil {
					continue
				}
				nreq.Header.Set("Authorization", "Bearer "+token)
				nresp, err := client.Do(nreq)
				if err != nil {
					continue
				}
				nbody, _ := io.ReadAll(nresp.Body)
				nresp.Body.Close()

				var nodeList k8sNodeList
				if err := json.Unmarshal(nbody, &nodeList); err != nil {
					continue
				}

				// Find ready nodes
				var readyNodes []string
				for _, node := range nodeList.Items {
					isReady := false
					for _, cond := range node.Status.Conditions {
						if cond.Type == "Ready" && cond.Status == "True" {
							isReady = true
							break
						}
					}
					if isReady {
						readyNodes = append(readyNodes, node.Metadata.Name)
					}
				}

				if len(readyNodes) == 0 {
					logger.Warn("No ready nodes found for scheduling")
					continue
				}

				// 3. Score candidate nodes using predictive plugin
				podInfo := &v1.PodInfo{
					Namespace: pod.Metadata.Namespace,
					Name:      pod.Metadata.Name,
				}

				bestNode := readyNodes[0]
				var maxScore int64 = -1

				for _, nodeName := range readyNodes {
					score, err := plugin.ScoreNode(ctx, podInfo, nodeName)
					if err != nil {
						score = 50
					}
					if score > maxScore {
						maxScore = score
						bestNode = nodeName
					}
				}

				// 4. Bind pod to chosen node
				binding := k8sBinding{
					APIVersion: "v1",
					Kind:       "Binding",
				}
				binding.Metadata.Name = pod.Metadata.Name
				binding.Metadata.Namespace = pod.Metadata.Namespace
				binding.Target.APIVersion = "v1"
				binding.Target.Kind = "Node"
				binding.Target.Name = bestNode

				bindData, _ := json.Marshal(binding)
				bindURL := fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s/binding", baseURL, pod.Metadata.Namespace, pod.Metadata.Name)

				breq, err := http.NewRequestWithContext(ctx, "POST", bindURL, bytes.NewReader(bindData))
				if err != nil {
					continue
				}
				breq.Header.Set("Authorization", "Bearer "+token)
				breq.Header.Set("Content-Type", "application/json")

				bresp, err := client.Do(breq)
				if err != nil {
					logger.Error("Failed to execute pod binding", "pod", pod.Metadata.Name, "err", err)
					continue
				}
				bresp.Body.Close()

				if bresp.StatusCode >= 200 && bresp.StatusCode < 300 {
					logger.Info("Pod bound successfully to node by predictive scheduler",
						"pod", pod.Metadata.Name,
						"namespace", pod.Metadata.Namespace,
						"node", bestNode,
						"score", maxScore)
				} else {
					logger.Warn("Binding rejected by Kubernetes API", "status", bresp.StatusCode)
				}
			}
		}
	}
}

func main() {
	var (
		metricsPort   int
		schedulerName string
		leaderElect   bool
	)

	flag.IntVar(&metricsPort, "metrics-port", 10259, "Port to expose Prometheus metrics and health check")
	flag.StringVar(&schedulerName, "scheduler-name", v1.SchedulerNamePredictive, "Name of the scheduler to claim pods for")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election for high availability")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("component", "predictive-scheduler-plugin")

	logger.Info("Starting Predictive Kubernetes Scheduler Plugin",
		"schedulerName", schedulerName,
		"metricsPort", metricsPort,
		"leaderElect", leaderElect)

	// Metrics endpoint
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	metricsServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", metricsPort),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Metrics server error", "err", err)
		}
	}()

	cache := scheduler.NewInMemoryScoreCache()
	plugin := scheduler.NewPlugin(cache, 1)

	logger.Info("Predictive scoring plugin registered successfully into scheduler framework",
		"plugin", plugin.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch active scheduling reconciler loop
	go runSchedulingReconciler(ctx, schedulerName, plugin, logger)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	<-stopCh

	logger.Info("Stopping scheduler plugin...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = metricsServer.Shutdown(shutdownCtx)
	logger.Info("Scheduler plugin stopped")
}
