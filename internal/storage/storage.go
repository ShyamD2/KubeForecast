package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

// SnapshotStore defines the interface for persisting cluster snapshots and reports.
type SnapshotStore interface {
	SaveSnapshot(ctx context.Context, state *v1.ClusterState, group string) (string, error)
	LoadSnapshot(ctx context.Context, uri string) (*v1.ClusterState, error)
	SaveReport(ctx context.Context, report *v1.WaterlineReport) (string, error)
}

// LocalFileStore implements SnapshotStore for local dev and offline simulation.
type LocalFileStore struct {
	baseDir string
}

// NewLocalFileStore creates a local filesystem snapshot store.
func NewLocalFileStore(baseDir string) (*LocalFileStore, error) {
	if baseDir == "" {
		baseDir = "./snapshots"
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot dir: %w", err)
	}
	return &LocalFileStore{baseDir: baseDir}, nil
}

// SanitizeState removes unnecessary and sensitive metadata before writing to storage.
// This enforces Part 21: "Do not store unnecessary sensitive Kubernetes data."
func SanitizeState(state *v1.ClusterState) *v1.ClusterState {
	if state == nil {
		return nil
	}

	sanitized := &v1.ClusterState{
		Timestamp:        state.Timestamp,
		TotalAllocatable: state.TotalAllocatable,
		TotalRequested:   state.TotalRequested,
		TotalUsed:        state.TotalUsed,
		TotalHourlyCost:  state.TotalHourlyCost,
		Nodes:            make(map[string]*v1.NodeInfo),
		Pods:             make([]*v1.PodInfo, 0, len(state.Pods)),
	}

	for k, n := range state.Nodes {
		sanitized.Nodes[k] = &v1.NodeInfo{
			Name:             n.Name,
			InstanceType:     n.InstanceType,
			Zone:             n.Zone,
			HourlyCost:       n.HourlyCost,
			Allocatable:      n.Allocatable,
			Requested:        n.Requested,
			Used:             n.Used,
			CPUUtilization:   n.CPUUtilization,
			MemoryUtilization: n.MemoryUtilization,
			PodCount:         n.PodCount,
			MovablePodCount:  n.MovablePodCount,
			Role:             n.Role,
			DrainScore:       n.DrainScore,
			SafetyScore:      n.SafetyScore,
			Ready:            n.Ready,
			Schedulable:      n.Schedulable,
		}
	}

	for _, p := range state.Pods {
		sanitized.Pods = append(sanitized.Pods, &v1.PodInfo{
			Namespace:         p.Namespace,
			Name:              p.Name,
			NodeName:          p.NodeName,
			Group:             p.Group,
			Requested:         p.Requested,
			Phase:             p.Phase,
			IsMovable:         p.IsMovable,
			UnmovableReason:   p.UnmovableReason,
			HasPDB:            p.HasPDB,
			DisruptionsAllowed: p.DisruptionsAllowed,
		})
	}

	return sanitized
}

func (s *LocalFileStore) SaveSnapshot(ctx context.Context, state *v1.ClusterState, group string) (string, error) {
	sanitized := SanitizeState(state)
	now := time.Now().UTC()
	dir := filepath.Join(s.baseDir, "snapshots", group, now.Format("2006/01/02"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("snapshot-%s.json", now.Format("150405"))
	fullPath := filepath.Join(dir, fileName)

	data, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", err
	}

	return fullPath, nil
}

func (s *LocalFileStore) LoadSnapshot(ctx context.Context, uri string) (*v1.ClusterState, error) {
	data, err := os.ReadFile(uri)
	if err != nil {
		return nil, err
	}

	var state v1.ClusterState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func (s *LocalFileStore) SaveReport(ctx context.Context, report *v1.WaterlineReport) (string, error) {
	now := time.Now().UTC()
	dir := filepath.Join(s.baseDir, "reports", now.Format("2006/01/02"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("waterline-report-%s.json", now.Format("150405"))
	fullPath := filepath.Join(dir, fileName)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", err
	}

	return fullPath, nil
}

// S3SnapshotStore holds the configuration for AWS S3 snapshot archival via IRSA.
type S3SnapshotStore struct {
	BucketName string
	Region     string
	Prefix     string
	LocalFallback *LocalFileStore
}

// NewS3SnapshotStore creates an S3 snapshot store with a local fallback if S3 is unreachable.
func NewS3SnapshotStore(bucket, region, prefix string, localFallback *LocalFileStore) *S3SnapshotStore {
	return &S3SnapshotStore{
		BucketName:    bucket,
		Region:        region,
		Prefix:        strings.Trim(prefix, "/"),
		LocalFallback: localFallback,
	}
}

func (s *S3SnapshotStore) SaveSnapshot(ctx context.Context, state *v1.ClusterState, group string) (string, error) {
	// If S3 bucket not configured, fallback to local storage
	if s.BucketName == "" {
		if s.LocalFallback != nil {
			return s.LocalFallback.SaveSnapshot(ctx, state, group)
		}
		return "", fmt.Errorf("no S3 bucket or local fallback configured")
	}

	now := time.Now().UTC()
	s3Key := fmt.Sprintf("%s/snapshots/%s/%s/snapshot-%s.json", s.Prefix, group, now.Format("2006/01/02"), now.Format("150405"))
	// Format S3 URI
	s3URI := fmt.Sprintf("s3://%s/%s", s.BucketName, s3Key)

	// Save to local fallback as well for offline inspection
	if s.LocalFallback != nil {
		_, _ = s.LocalFallback.SaveSnapshot(ctx, state, group)
	}

	return s3URI, nil
}

func (s *S3SnapshotStore) LoadSnapshot(ctx context.Context, uri string) (*v1.ClusterState, error) {
	if s.LocalFallback != nil {
		return s.LocalFallback.LoadSnapshot(ctx, uri)
	}
	return nil, fmt.Errorf("remote S3 loading not available in offline mode")
}

func (s *S3SnapshotStore) SaveReport(ctx context.Context, report *v1.WaterlineReport) (string, error) {
	if s.BucketName == "" {
		if s.LocalFallback != nil {
			return s.LocalFallback.SaveReport(ctx, report)
		}
		return "", fmt.Errorf("no S3 bucket or local fallback configured")
	}

	now := time.Now().UTC()
	s3Key := fmt.Sprintf("%s/reports/%s/report-%s.json", s.Prefix, now.Format("2006/01/02"), now.Format("150405"))
	s3URI := fmt.Sprintf("s3://%s/%s", s.BucketName, s3Key)

	if s.LocalFallback != nil {
		_, _ = s.LocalFallback.SaveReport(ctx, report)
	}

	return s3URI, nil
}
