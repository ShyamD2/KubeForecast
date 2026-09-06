package v1

import (
	"time"
)

// ExperimentGroup defines the experiment categorization for a workload or node.
type ExperimentGroup string

const (
	GroupControl   ExperimentGroup = "control"
	GroupTreatment ExperimentGroup = "treatment"
	GroupUnknown   ExperimentGroup = "unknown"
)

// Label and Annotation constants used across the system.
const (
	LabelExperimentGroup = "predictive-scheduler/group"
	LabelWaterlineRole   = "predictive-scheduler/role"
	LabelInstanceType    = "node.kubernetes.io/instance-type"
	LabelTopologyZone    = "topology.kubernetes.io/zone"

	AnnotationPredictedDrain = "predictive-scheduler.io/predicted-drain"
	AnnotationDrainScore     = "predictive-scheduler.io/drain-score"
	AnnotationSafetyScore    = "predictive-scheduler.io/safety-score"
	AnnotationAdmittedAt     = "predictive-scheduler.io/admitted-at"
	AnnotationRoutingReason  = "predictive-scheduler.io/routing-reason"
	AnnotationEvictionTarget = "predictive-scheduler.io/eviction-target"

	SchedulerNamePredictive = "predictive-scheduler"
	SchedulerNameDefault    = "default-scheduler"
)

// NodeRole indicates whether a node is classified as safe or as a drain candidate.
type NodeRole string

const (
	NodeRoleSafe      NodeRole = "safe"
	NodeRoleCandidate NodeRole = "candidate"
	NodeRoleNeutral   NodeRole = "neutral"
)

// ResourceQuantities holds normalized CPU (millicores) and Memory (bytes).
type ResourceQuantities struct {
	CPU    int64 `json:"cpu"`    // millicores (1 core = 1000m)
	Memory int64 `json:"memory"` // bytes
}

// NodeInfo captures all relevant scheduling and cost attributes of a node.
type NodeInfo struct {
	Name              string             `json:"name"`
	InstanceType      string             `json:"instanceType"`
	Zone              string             `json:"zone"`
	HourlyCost        float64            `json:"hourlyCost"`
	Allocatable       ResourceQuantities `json:"allocatable"`
	Requested         ResourceQuantities `json:"requested"`
	Used              ResourceQuantities `json:"used"`
	CPUUtilization    float64            `json:"cpuUtilization"`    // 0.0 - 1.0 (used / allocatable)
	MemoryUtilization float64            `json:"memoryUtilization"` // 0.0 - 1.0 (used / allocatable)
	CPULoadRatio      float64            `json:"cpuLoadRatio"`      // 0.0 - 1.0 (requested / allocatable)
	MemoryLoadRatio   float64            `json:"memoryLoadRatio"`   // 0.0 - 1.0 (requested / allocatable)
	PodCount          int                `json:"podCount"`
	MovablePodCount   int                `json:"movablePodCount"`
	Role              NodeRole           `json:"role"`
	DrainScore        float64            `json:"drainScore"`  // 0.0 - 100.0 (higher = more likely candidate for drain)
	SafetyScore       float64            `json:"safetyScore"` // 0.0 - 100.0 (higher = safer to schedule on)
	Ready             bool               `json:"ready"`
	Schedulable       bool               `json:"schedulable"`
	Taints            []TaintInfo        `json:"taints,omitempty"`
	Labels            map[string]string  `json:"labels,omitempty"`
}

// TaintInfo holds node taint information.
type TaintInfo struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

// PodInfo captures workload resource demands, mobility constraints, and experiment tracking.
type PodInfo struct {
	Namespace          string             `json:"namespace"`
	Name               string             `json:"name"`
	UID                string             `json:"uid"`
	NodeName           string             `json:"nodeName"`
	Group              ExperimentGroup    `json:"group"`
	Requested          ResourceQuantities `json:"requested"`
	Limits             ResourceQuantities `json:"limits"`
	Used               ResourceQuantities `json:"used,omitempty"`
	Phase              string             `json:"phase"`
	IsMovable          bool               `json:"isMovable"`
	UnmovableReason    string             `json:"unmovableReason,omitempty"`
	HasPDB             bool               `json:"hasPdb"`
	DisruptionsAllowed int32              `json:"disruptionsAllowed"`
	Labels             map[string]string  `json:"labels,omitempty"`
	Annotations        map[string]string  `json:"annotations,omitempty"`
	NodeSelector       map[string]string  `json:"nodeSelector,omitempty"`
	Tolerations        []TaintInfo        `json:"tolerations,omitempty"`
	CreatedAt          time.Time          `json:"createdAt"`
}

// ClusterState represents the complete point-in-time state of the Kubernetes cluster.
type ClusterState struct {
	Timestamp        time.Time            `json:"timestamp"`
	Nodes            map[string]*NodeInfo `json:"nodes"`
	Pods             []*PodInfo           `json:"pods"`
	TotalAllocatable ResourceQuantities   `json:"totalAllocatable"`
	TotalRequested   ResourceQuantities   `json:"totalRequested"`
	TotalUsed        ResourceQuantities   `json:"totalUsed"`
	TotalHourlyCost  float64              `json:"totalHourlyCost"`
}

// WaterlineConfig holds tuning parameters and weights for the Waterline scoring engine.
type WaterlineConfig struct {
	CPUWeight                  float64 `json:"cpuWeight" yaml:"cpuWeight"`                                   // e.g. 0.30
	MemoryWeight               float64 `json:"memoryWeight" yaml:"memoryWeight"`                             // e.g. 0.30
	ConsolidationWeight        float64 `json:"consolidationWeight" yaml:"consolidationWeight"`               // e.g. 0.20
	DisruptionWeight           float64 `json:"disruptionWeight" yaml:"disruptionWeight"`                     // e.g. 0.15
	HistoricalWeight           float64 `json:"historicalWeight" yaml:"historicalWeight"`                     // e.g. 0.05
	TargetUtilizationThreshold float64 `json:"targetUtilizationThreshold" yaml:"targetUtilizationThreshold"` // e.g. 0.75
	CandidateDrainThreshold    float64 `json:"candidateDrainThreshold" yaml:"candidateDrainThreshold"`       // e.g. 0.40
	MinNodesFloor              int     `json:"minNodesFloor" yaml:"minNodesFloor"`                           // e.g. 2
}

// DefaultWaterlineConfig returns sensible, production-tested default weights.
func DefaultWaterlineConfig() WaterlineConfig {
	return WaterlineConfig{
		CPUWeight:                  0.30,
		MemoryWeight:               0.30,
		ConsolidationWeight:        0.20,
		DisruptionWeight:           0.15,
		HistoricalWeight:           0.05,
		TargetUtilizationThreshold: 0.75,
		CandidateDrainThreshold:    0.40,
		MinNodesFloor:              2,
	}
}

// Prediction stores the computed drain probability and transparent explanations for a node.
type Prediction struct {
	NodeName       string    `json:"nodeName"`
	Probability    float64   `json:"probability"` // 0.0 - 1.0
	Confidence     float64   `json:"confidence"`  // 0.0 - 1.0
	DrainCandidate bool      `json:"drainCandidate"`
	Reasons        []string  `json:"reasons"`
	Timestamp      time.Time `json:"timestamp"`
}

// DrainCandidateInfo provides detailed metrics on candidate nodes selected for drain.
type DrainCandidateInfo struct {
	NodeName      string             `json:"nodeName"`
	DrainScore    float64            `json:"drainScore"`
	SafetyScore   float64            `json:"safetyScore"`
	Allocatable   ResourceQuantities `json:"allocatable"`
	Requested     ResourceQuantities `json:"requested"`
	MovablePods   []string           `json:"movablePods"`
	UnmovablePods []string           `json:"unmovablePods"`
	Probability   float64            `json:"probability"`
	Confidence    float64            `json:"confidence"`
	Reasons       []string           `json:"reasons"`
}

// WaterlineReport captures the outcome of a simulation or waterline evaluation.
type WaterlineReport struct {
	Timestamp                        time.Time            `json:"timestamp"`
	WaterlineLevel                   float64              `json:"waterlineLevel"` // aggregate target utilization
	SafeNodes                        []string             `json:"safeNodes"`
	DrainCandidates                  []DrainCandidateInfo `json:"drainCandidates"`
	NeutralNodes                     []string             `json:"neutralNodes"`
	TotalNodes                       int                  `json:"totalNodes"`
	ProjectedNodesAfterConsolidation int                  `json:"projectedNodesAfterConsolidation"`
	CurrentHourlyCost                float64              `json:"currentHourlyCost"`
	ProjectedHourlyCost              float64              `json:"projectedHourlyCost"`
	PotentialHourlySavings           float64              `json:"potentialHourlySavings"`
	CurrentClusterWaste              WasteReport          `json:"currentClusterWaste"`
	ProjectedClusterWaste            WasteReport          `json:"projectedClusterWaste"`
}

// WasteReport decomposes resource waste across dimensions.
type WasteReport struct {
	CPUWastePercent       float64 `json:"cpuWastePercent"`
	MemoryWastePercent    float64 `json:"memoryWastePercent"`
	CompositeWastePercent float64 `json:"compositeWastePercent"`
	TotalRequestedCPU     int64   `json:"totalRequestedCpu"`
	TotalUsedCPU          int64   `json:"totalUsedCpu"`
	TotalRequestedMemory  int64   `json:"totalRequestedMemory"`
	TotalUsedMemory       int64   `json:"totalUsedMemory"`
}

// EvictionConfig controls the safety guards and rate limits of the eviction controller.
type EvictionConfig struct {
	Enabled                 bool          `json:"enabled" yaml:"enabled"`
	MaxPodsPerHour          int           `json:"maxPodsPerHour" yaml:"maxPodsPerHour"`
	MaxConcurrentEvictions  int           `json:"maxConcurrentEvictions" yaml:"maxConcurrentEvictions"`
	MinAvailableCapacityPct float64       `json:"minAvailableCapacityPct" yaml:"minAvailableCapacityPct"` // e.g. 0.20 (20%)
	Cooldown                time.Duration `json:"cooldown" yaml:"cooldown"`
	DryRun                  bool          `json:"dryRun" yaml:"dryRun"`
}

// DefaultEvictionConfig returns safe-by-default eviction parameters (disabled by default).
func DefaultEvictionConfig() EvictionConfig {
	return EvictionConfig{
		Enabled:                 false,
		MaxPodsPerHour:          2,
		MaxConcurrentEvictions:  1,
		MinAvailableCapacityPct: 0.20,
		Cooldown:                15 * time.Minute,
		DryRun:                  false,
	}
}

// EvictionTarget describes a specific pod selected for safe eviction.
type EvictionTarget struct {
	Namespace       string             `json:"namespace"`
	Name            string             `json:"name"`
	NodeName        string             `json:"nodeName"`
	DestinationNode string             `json:"destinationNode"`
	Requested       ResourceQuantities `json:"requested"`
	Reason          string             `json:"reason"`
}

// EvictionPlan records the safety validation and dry-run feasibility for a proposed drain.
type EvictionPlan struct {
	Timestamp           time.Time        `json:"timestamp"`
	CandidateNode       string           `json:"candidateNode"`
	Targets             []EvictionTarget `json:"targets"`
	TotalReclaimableCPU int64            `json:"totalReclaimableCpu"`
	TotalReclaimableMem int64            `json:"totalReclaimableMem"`
	Feasible            bool             `json:"feasible"`
	SafetyErrors        []string         `json:"safetyErrors,omitempty"`
}

// PricingConfig maps AWS EC2 instance types to hourly on-demand or spot prices ($ USD).
type PricingConfig struct {
	Prices       map[string]float64 `json:"prices" yaml:"prices"`
	DefaultPrice float64            `json:"defaultPrice" yaml:"defaultPrice"`
}

// DefaultPricingConfig returns standard US-East-1 AWS on-demand reference pricing.
func DefaultPricingConfig() PricingConfig {
	return PricingConfig{
		Prices: map[string]float64{
			"m5.large":   0.096,
			"m5.xlarge":  0.192,
			"m5.2xlarge": 0.384,
			"c5.large":   0.085,
			"c5.xlarge":  0.170,
			"c5.2xlarge": 0.340,
			"r5.large":   0.126,
			"r5.xlarge":  0.252,
			"t3.medium":  0.0416,
			"t3.large":   0.0832,
			"t3.xlarge":  0.1664,
		},
		DefaultPrice: 0.096, // fallback to m5.large equivalent
	}
}
