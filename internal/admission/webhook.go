package admission

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/metrics"
)

// AdmissionReviewRequest represents the relevant subset of k8s.io/api/admission/v1 AdmissionReview.
type AdmissionReviewRequest struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Request    struct {
		UID       string          `json:"uid"`
		Namespace string          `json:"namespace"`
		Name      string          `json:"name"`
		Operation string          `json:"operation"`
		Object    json.RawMessage `json:"object"`
	} `json:"request"`
}

// AdmissionReviewResponse defines the admission response returned to API server.
type AdmissionReviewResponse struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Response   struct {
		UID       string      `json:"uid"`
		Allowed   bool        `json:"allowed"`
		PatchType string      `json:"patchType,omitempty"`
		Patch     []byte      `json:"patch,omitempty"`
		Result    *StatusInfo `json:"status,omitempty"`
	} `json:"response"`
}

type StatusInfo struct {
	Message string `json:"message,omitempty"`
	Code    int32  `json:"code,omitempty"`
}

// JSONPatchOperation represents an RFC 6902 JSON patch item.
type JSONPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// PodSpecMinimal is used to unmarshal the incoming Pod without pulling the full k8s api schema.
type PodSpecMinimal struct {
	Metadata struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		SchedulerName string `json:"schedulerName"`
	} `json:"spec"`
}

// Handler handles admission webhook mutate requests.
type Handler struct {
	logger *slog.Logger
}

// NewHandler creates a webhook mutation handler.
func NewHandler(logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{logger: logger}
}

// MutatePod processes pod creation and routes between default-scheduler and predictive-scheduler.
func (h *Handler) MutatePod(req *AdmissionReviewRequest) *AdmissionReviewResponse {
	start := time.Now()
	resp := &AdmissionReviewResponse{
		Kind:       req.Kind,
		APIVersion: req.APIVersion,
	}
	resp.Response.UID = req.Request.UID
	resp.Response.Allowed = true // Always allow by default for safety

	var pod PodSpecMinimal
	if err := json.Unmarshal(req.Request.Object, &pod); err != nil {
		h.logger.Error("failed to unmarshal pod in admission request", "err", err, "uid", req.Request.UID)
		metrics.WebhookRequestsTotal.WithLabelValues("error", "unmarshal_failed").Inc()
		return resp
	}

	ns := strings.ToLower(req.Request.Namespace)
	group := v1.GroupUnknown

	// Determine experiment group from namespace naming or pod labels
	if pod.Metadata.Labels != nil && pod.Metadata.Labels[v1.LabelExperimentGroup] != "" {
		group = v1.ExperimentGroup(pod.Metadata.Labels[v1.LabelExperimentGroup])
	} else if strings.Contains(ns, "treatment") {
		group = v1.GroupTreatment
	} else if strings.Contains(ns, "control") {
		group = v1.GroupControl
	}

	patches := make([]JSONPatchOperation, 0)
	nowISO := time.Now().UTC().Format(time.RFC3339)

	// Ensure annotations map exists
	if pod.Metadata.Annotations == nil {
		patches = append(patches, JSONPatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations",
			Value: map[string]string{},
		})
	}

	// Always record admission timestamp
	patches = append(patches, JSONPatchOperation{
		Op:    "add",
		Path:  "/metadata/annotations/" + escapeJSONPointer(v1.AnnotationAdmittedAt),
		Value: nowISO,
	})

	switch group {
	case v1.GroupTreatment:
		// Route to predictive scheduler
		patches = append(patches, JSONPatchOperation{
			Op:    "add",
			Path:  "/spec/schedulerName",
			Value: v1.SchedulerNamePredictive,
		})
		patches = append(patches, JSONPatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations/" + escapeJSONPointer(v1.AnnotationRoutingReason),
			Value: "Routed to predictive-scheduler for treatment experiment group",
		})
		metrics.WebhookRequestsTotal.WithLabelValues(string(v1.GroupTreatment), "mutated").Inc()
		metrics.WebhookLatencySeconds.WithLabelValues(string(v1.GroupTreatment)).Observe(time.Since(start).Seconds())

	case v1.GroupControl:
		// Explicitly confirm default-scheduler for control group
		if pod.Spec.SchedulerName == "" {
			patches = append(patches, JSONPatchOperation{
				Op:    "add",
				Path:  "/spec/schedulerName",
				Value: v1.SchedulerNameDefault,
			})
		}
		patches = append(patches, JSONPatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations/" + escapeJSONPointer(v1.AnnotationRoutingReason),
			Value: "Assigned default-scheduler for control baseline group",
		})
		metrics.WebhookRequestsTotal.WithLabelValues(string(v1.GroupControl), "control_retained").Inc()
		metrics.WebhookLatencySeconds.WithLabelValues(string(v1.GroupControl)).Observe(time.Since(start).Seconds())

	default:
		// Non-experiment workload: do not mutate schedulerName
		metrics.WebhookRequestsTotal.WithLabelValues("non_experiment", "unmodified").Inc()
		metrics.WebhookLatencySeconds.WithLabelValues("non_experiment").Observe(time.Since(start).Seconds())
	}

	if len(patches) > 0 {
		patchBytes, err := json.Marshal(patches)
		if err == nil {
			resp.Response.PatchType = "JSONPatch"
			resp.Response.Patch = patchBytes
		}
	}

	return resp
}

func escapeJSONPointer(str string) string {
	str = strings.ReplaceAll(str, "~", "~0")
	return strings.ReplaceAll(str, "/", "~1")
}

// Server provides the TLS HTTP server for the admission webhook.
type Server struct {
	httpServer *http.Server
	handler    *Handler
	logger     *slog.Logger
}

// NewServer initializes the webhook server.
func NewServer(addr string, tlsConfig *tls.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	handler := NewHandler(logger)
	mux := http.NewServeMux()

	s := &Server{
		handler: handler,
		logger:  logger,
	}

	mux.HandleFunc("/mutate", s.handleMutate)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s
}

func (s *Server) handleMutate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req AdmissionReviewRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid AdmissionReview JSON", http.StatusBadRequest)
		return
	}

	resp := s.handler.MutatePod(&req)
	respBytes, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// Start runs the webhook server.
func (s *Server) Start(certFile, keyFile string) error {
	s.logger.Info("Starting admission webhook server", "addr", s.httpServer.Addr)
	if certFile != "" && keyFile != "" {
		return s.httpServer.ListenAndServeTLS(certFile, keyFile)
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully terminates the webhook server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down admission webhook server")
	return s.httpServer.Shutdown(ctx)
}
