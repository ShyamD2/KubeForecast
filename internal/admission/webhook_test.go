package admission

import (
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestMutatePod_TreatmentGroup(t *testing.T) {
	handler := NewHandler(nil)

	reqJSON := `{
		"kind": "AdmissionReview",
		"apiVersion": "admission.k8s.io/v1",
		"request": {
			"uid": "12345",
			"namespace": "experiment-treatment",
			"object": {
				"metadata": {
					"name": "worker-pod",
					"namespace": "experiment-treatment"
				},
				"spec": {}
			}
		}
	}`

	var req AdmissionReviewRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	resp := handler.MutatePod(&req)
	if !resp.Response.Allowed {
		t.Fatalf("expected allowed true")
	}

	patchStr := string(resp.Response.Patch)
	if !strings.Contains(patchStr, v1.SchedulerNamePredictive) {
		t.Errorf("expected patch to inject schedulerName %s, got: %s", v1.SchedulerNamePredictive, patchStr)
	}
}

func TestMutatePod_ControlGroup(t *testing.T) {
	handler := NewHandler(nil)

	reqJSON := `{
		"kind": "AdmissionReview",
		"apiVersion": "admission.k8s.io/v1",
		"request": {
			"uid": "67890",
			"namespace": "experiment-control",
			"object": {
				"metadata": {
					"name": "baseline-pod",
					"namespace": "experiment-control"
				},
				"spec": {}
			}
		}
	}`

	var req AdmissionReviewRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	resp := handler.MutatePod(&req)
	if !resp.Response.Allowed {
		t.Fatalf("expected allowed true")
	}

	patchStr := string(resp.Response.Patch)
	if !strings.Contains(patchStr, v1.SchedulerNameDefault) {
		t.Errorf("expected patch to retain default-scheduler for control, got: %s", patchStr)
	}
}

func TestMutatePod_FailSafe(t *testing.T) {
	handler := NewHandler(nil)

	// Malformed object payload
	var req AdmissionReviewRequest
	req.Request.UID = "bad-request"
	req.Request.Object = []byte(`invalid json`)

	resp := handler.MutatePod(&req)
	// Must still allow so workload creation does not fail
	if !resp.Response.Allowed {
		t.Errorf("webhook must fail open to prevent cluster unavailability")
	}
}
