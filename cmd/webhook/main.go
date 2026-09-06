package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/admission"
)

func main() {
	var (
		port        int
		metricsPort int
		certFile    string
		keyFile     string
	)

	flag.IntVar(&port, "port", 8443, "Port to listen on for admission requests")
	flag.IntVar(&metricsPort, "metrics-port", 8080, "Port to expose Prometheus metrics and health checks")
	flag.StringVar(&certFile, "tls-cert-file", "/etc/webhook/certs/tls.crt", "File containing TLS certificate")
	flag.StringVar(&keyFile, "tls-key-file", "/etc/webhook/certs/tls.key", "File containing TLS private key")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("component", "predictive-admission-webhook")

	logger.Info("Starting Predictive Admission Webhook",
		"port", port,
		"metricsPort", metricsPort,
		"certFile", certFile)

	// Metrics server
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	metricsServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", metricsPort),
		Handler:      metricsMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Metrics server error", "err", err)
		}
	}()

	// TLS Webhook server
	var tlsConf *tls.Config
	if _, err := os.Stat(certFile); err == nil {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			logger.Error("Failed to load TLS key pair, running non-TLS fallback for local dev", "err", err)
		} else {
			tlsConf = &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}
		}
	} else {
		logger.Warn("TLS certificates not found on disk, running non-TLS fallback for local test")
	}

	webhookServer := admission.NewServer(fmt.Sprintf(":%d", port), tlsConf, logger)

	go func() {
		if err := webhookServer.Start(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			logger.Error("Webhook server terminated unexpectedly", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown handling
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	<-stopCh

	logger.Info("Shutdown signal received, draining active requests...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = webhookServer.Shutdown(ctx)
	_ = metricsServer.Shutdown(ctx)
	logger.Info("Admission Webhook gracefully stopped")
}
