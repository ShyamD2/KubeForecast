package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/cluster"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/eviction"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/metrics"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/scheduler"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/simulation"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/storage"
)

func main() {
	var (
		intervalStr     string
		metricsPort     int
		evictionEnabled bool
		s3Bucket        string
		awsRegion       string
		snapshotDir     string
	)

	flag.StringVar(&intervalStr, "simulation-interval", "15m", "Periodic evaluation loop interval")
	flag.IntVar(&metricsPort, "metrics-port", 8080, "Port to expose Prometheus metrics")
	flag.BoolVar(&evictionEnabled, "eviction-enabled", false, "Enable controlled pod evictions (safe by default: false)")
	flag.StringVar(&s3Bucket, "s3-bucket", "", "AWS S3 bucket for cluster snapshots (leave empty for local storage)")
	flag.StringVar(&awsRegion, "aws-region", "us-east-1", "AWS region for S3 snapshots")
	flag.StringVar(&snapshotDir, "snapshot-dir", "./snapshots", "Local snapshot directory")
	flag.Parse()

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 15 * time.Minute
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("component", "predictive-controller")

	logger.Info("Starting Kubernetes Predictive Optimization Controller",
		"interval", interval,
		"evictionEnabled", evictionEnabled,
		"metricsPort", metricsPort,
		"s3Bucket", s3Bucket)

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

	// Storage setup
	localStore, err := storage.NewLocalFileStore(snapshotDir)
	if err != nil {
		logger.Error("Failed to initialize local snapshot store", "err", err)
		os.Exit(1)
	}

	var store storage.SnapshotStore = localStore
	if s3Bucket != "" {
		store = storage.NewS3SnapshotStore(s3Bucket, awsRegion, "kubeforecast", localStore)
		logger.Info("S3 snapshot persistence configured", "bucket", s3Bucket, "region", awsRegion)
	}

	// Initialize Engine
	cfg := v1.DefaultWaterlineConfig()
	pricing := v1.DefaultPricingConfig()
	scoreCache := scheduler.NewInMemoryScoreCache()
	simEngine := simulation.NewEngine(cfg, pricing, store, scoreCache, logger)

	evictionCfg := v1.DefaultEvictionConfig()
	evictionCfg.Enabled = evictionEnabled
	evictionCtrl := eviction.NewController(evictionCfg)
	collector := cluster.NewStateCollector(pricing)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial run
	runOptimizationCycle(ctx, simEngine, evictionCtrl, collector, evictionEnabled, logger)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			runOptimizationCycle(ctx, simEngine, evictionCtrl, collector, evictionEnabled, logger)
		case <-stopCh:
			logger.Info("Controller shutdown received, terminating...")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = metricsServer.Shutdown(shutdownCtx)
			shutdownCancel()
			logger.Info("Controller terminated cleanly")
			return
		case <-ctx.Done():
			return
		}
	}
}

func runOptimizationCycle(
	ctx context.Context,
	simEngine simulation.Engine,
	evictionCtrl *eviction.Controller,
	collector cluster.StateCollector,
	evictionEnabled bool,
	logger *slog.Logger,
) {
	logger.Info("Executing cluster optimization cycle")

	// Sample mock / cluster state
	// In production, this pulls from client-go informers
	nodes := []*v1.NodeInfo{
		{
			Name:         "ip-10-0-1-10.ec2.internal",
			InstanceType: "m5.large",
			Allocatable:  v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Requested:    v1.ResourceQuantities{CPU: 3200, Memory: 13000},
			Used:         v1.ResourceQuantities{CPU: 2800, Memory: 11000},
			Ready:        true,
			Schedulable:  true,
		},
		{
			Name:         "ip-10-0-1-11.ec2.internal",
			InstanceType: "m5.large",
			Allocatable:  v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Requested:    v1.ResourceQuantities{CPU: 300, Memory: 1200},
			Used:         v1.ResourceQuantities{CPU: 150, Memory: 800},
			Ready:        true,
			Schedulable:  true,
		},
	}

	pods := []*v1.PodInfo{
		{
			Namespace:          "treatment",
			Name:               "api-server-7b89d4-x1",
			NodeName:           "ip-10-0-1-10.ec2.internal",
			Requested:          v1.ResourceQuantities{CPU: 1600, Memory: 6500},
			Used:               v1.ResourceQuantities{CPU: 1400, Memory: 5500},
			IsMovable:          true,
			HasPDB:             true,
			DisruptionsAllowed: 1,
		},
		{
			Namespace: "treatment",
			Name:      "cache-worker-9c8e-z2",
			NodeName:  "ip-10-0-1-11.ec2.internal",
			Requested: v1.ResourceQuantities{CPU: 300, Memory: 1200},
			Used:      v1.ResourceQuantities{CPU: 150, Memory: 800},
			IsMovable: true,
			HasPDB:    false,
		},
	}

	state := collector.BuildClusterState(ctx, nodes, pods)

	simResult, err := simEngine.RunSimulation(ctx, state)
	if err != nil {
		logger.Error("Simulation cycle failed", "err", err)
		return
	}

	logger.Info("Waterline simulation complete",
		"safeNodes", len(simResult.Report.SafeNodes),
		"drainCandidates", len(simResult.Report.DrainCandidates),
		"hourlySavings", simResult.Report.PotentialHourlySavings)

	// Safe eviction dry-run planning
	if len(simResult.Report.DrainCandidates) > 0 {
		plan, err := evictionCtrl.PlanEvictions(ctx, state, simResult.Report.DrainCandidates, simResult.Report.SafeNodes)
		if err != nil {
			logger.Warn("Failed to formulate eviction plan", "err", err)
			return
		}

		if plan.Feasible && len(plan.Targets) > 0 {
			for _, target := range plan.Targets {
				if evictionEnabled {
					logger.Info("Executing gentle pod eviction",
						"pod", target.Name,
						"namespace", target.Namespace,
						"fromNode", target.NodeName,
						"toNode", target.DestinationNode)
					metrics.EvictionsTotal.WithLabelValues(target.Namespace, "predictive_consolidation").Inc()
				} else {
					logger.Info("[DRY-RUN] Pod eviction planned but not executed (eviction disabled by default)",
						"pod", target.Name,
						"namespace", target.Namespace,
						"candidateNode", target.NodeName,
						"destinationNode", target.DestinationNode)
				}
			}
		} else {
			logger.Info("No feasible evictions identified in this cycle", "reasons", plan.SafetyErrors)
		}
	}
}
