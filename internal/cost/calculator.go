package cost

import (
	"fmt"
	"math"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

// Calculator provides FinOps accounting for cluster costs, multi-resource waste, and savings deltas.
type Calculator interface {
	CalculateNodeCost(instanceType string) float64
	CalculateClusterHourlyCost(nodes []*v1.NodeInfo) float64
	CalculateWaste(nodes []*v1.NodeInfo, pods []*v1.PodInfo) *GroupWasteSummary
	CalculateExperimentSavings(controlNodes, treatmentNodes []*v1.NodeInfo, duration time.Duration) *SavingsSummary
}

// GroupWasteSummary captures multi-dimensional waste for a group of nodes and pods.
type GroupWasteSummary struct {
	TotalAllocatableCPU int64   `json:"totalAllocatableCpu"`
	TotalAllocatableMem int64   `json:"totalAllocatableMem"`
	TotalRequestedCPU   int64   `json:"totalRequestedCpu"`
	TotalRequestedMem   int64   `json:"totalRequestedMem"`
	TotalUsedCPU        int64   `json:"totalUsedCpu"`
	TotalUsedMem        int64   `json:"totalUsedMem"`
	CPUWastePct         float64 `json:"cpuWastePct"`
	MemoryWastePct      float64 `json:"memoryWastePct"`
	StructuralWastePct  float64 `json:"structuralWastePct"` // Unallocated node capacity + unrequested usage gap
	HourlyCost          float64 `json:"hourlyCost"`
}

// SavingsSummary details the comparative financial and resource differences between Control and Treatment.
type SavingsSummary struct {
	Duration              time.Duration      `json:"duration"`
	ControlCost           float64            `json:"controlCost"`
	TreatmentCost         float64            `json:"treatmentCost"`
	AbsoluteSavings       float64            `json:"absoluteSavings"`
	SavingsPercent        float64            `json:"savingsPercent"`
	ControlWaste          *GroupWasteSummary `json:"controlWaste"`
	TreatmentWaste        *GroupWasteSummary `json:"treatmentWaste"`
	WasteReductionPercent float64            `json:"wasteReductionPercent"`
	Notes                 string             `json:"notes"`
}

type costCalculator struct {
	pricing v1.PricingConfig
}

// NewCalculator constructs a cost calculator with configurable pricing.
func NewCalculator(pricing v1.PricingConfig) Calculator {
	return &costCalculator{pricing: pricing}
}

// CalculateNodeCost returns the hourly on-demand cost for an instance type.
func (c *costCalculator) CalculateNodeCost(instanceType string) float64 {
	if price, ok := c.pricing.Prices[instanceType]; ok {
		return price
	}
	return c.pricing.DefaultPrice
}

// CalculateClusterHourlyCost sums the hourly cost of all active nodes.
func (c *costCalculator) CalculateClusterHourlyCost(nodes []*v1.NodeInfo) float64 {
	var total float64
	for _, n := range nodes {
		if n.HourlyCost > 0 {
			total += n.HourlyCost
		} else {
			total += c.CalculateNodeCost(n.InstanceType)
		}
	}
	return math.Round(total*1000) / 1000
}

// CalculateWaste evaluates multi-dimensional resource waste across CPU and memory.
// Engineering Note: Measuring CPU waste alone is inherently misleading in Kubernetes.
// Modern cloud workloads (Java JVMs, Redis, Kafka, in-memory caches) are frequently memory-bound.
// When memory reaches 90% allocation, CPU may be at 15%. If an optimization engine only observes CPU,
// it might flag the node for aggressive consolidation, triggering OOM-kills or eviction cascades.
// We calculate CPU waste, Memory waste, and Composite Structural Waste independently.
func (c *costCalculator) CalculateWaste(nodes []*v1.NodeInfo, pods []*v1.PodInfo) *GroupWasteSummary {
	var allocCPU, allocMem, reqCPU, reqMem, usedCPU, usedMem int64
	for _, n := range nodes {
		allocCPU += n.Allocatable.CPU
		allocMem += n.Allocatable.Memory
		reqCPU += n.Requested.CPU
		reqMem += n.Requested.Memory
		usedCPU += n.Used.CPU
		usedMem += n.Used.Memory
	}

	cpuWaste := 0.0
	if reqCPU > 0 {
		diff := reqCPU - usedCPU
		if diff > 0 {
			cpuWaste = (float64(diff) / float64(reqCPU)) * 100.0
		}
	}

	memWaste := 0.0
	if reqMem > 0 {
		diff := reqMem - usedMem
		if diff > 0 {
			memWaste = (float64(diff) / float64(reqMem)) * 100.0
		}
	}

	// Structural waste: capacity provisioned at node level that is never utilized
	var structuralWaste float64
	if allocCPU > 0 && allocMem > 0 {
		unutilizedCPU := float64(allocCPU-usedCPU) / float64(allocCPU)
		unutilizedMem := float64(allocMem-usedMem) / float64(allocMem)
		structuralWaste = ((unutilizedCPU + unutilizedMem) / 2.0) * 100.0
	}

	hourlyCost := c.CalculateClusterHourlyCost(nodes)

	return &GroupWasteSummary{
		TotalAllocatableCPU: allocCPU,
		TotalAllocatableMem: allocMem,
		TotalRequestedCPU:   reqCPU,
		TotalRequestedMem:   reqMem,
		TotalUsedCPU:        usedCPU,
		TotalUsedMem:        usedMem,
		CPUWastePct:         math.Round(cpuWaste*100) / 100,
		MemoryWastePct:      math.Round(memWaste*100) / 100,
		StructuralWastePct:  math.Round(structuralWaste*100) / 100,
		HourlyCost:          hourlyCost,
	}
}

// CalculateExperimentSavings compares Control vs Treatment groups over an experiment window.
func (c *costCalculator) CalculateExperimentSavings(controlNodes, treatmentNodes []*v1.NodeInfo, duration time.Duration) *SavingsSummary {
	controlWaste := c.CalculateWaste(controlNodes, nil)
	treatmentWaste := c.CalculateWaste(treatmentNodes, nil)

	hours := duration.Hours()
	if hours <= 0 {
		hours = 1.0
	}

	controlTotalCost := controlWaste.HourlyCost * hours
	treatmentTotalCost := treatmentWaste.HourlyCost * hours
	absoluteSavings := controlTotalCost - treatmentTotalCost

	savingsPct := 0.0
	if controlTotalCost > 0 {
		savingsPct = (absoluteSavings / controlTotalCost) * 100.0
	}

	wasteDelta := controlWaste.StructuralWastePct - treatmentWaste.StructuralWastePct
	wasteReductionPct := 0.0
	if controlWaste.StructuralWastePct > 0 {
		wasteReductionPct = (wasteDelta / controlWaste.StructuralWastePct) * 100.0
	}

	notes := fmt.Sprintf("Measured over %v duration with %d control nodes and %d treatment nodes.", duration, len(controlNodes), len(treatmentNodes))

	return &SavingsSummary{
		Duration:              duration,
		ControlCost:           math.Round(controlTotalCost*100) / 100,
		TreatmentCost:         math.Round(treatmentTotalCost*100) / 100,
		AbsoluteSavings:       math.Round(absoluteSavings*100) / 100,
		SavingsPercent:        math.Round(savingsPct*100) / 100,
		ControlWaste:          controlWaste,
		TreatmentWaste:        treatmentWaste,
		WasteReductionPercent: math.Round(wasteReductionPct*100) / 100,
		Notes:                 notes,
	}
}
