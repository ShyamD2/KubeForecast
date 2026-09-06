package cost

import (
	"testing"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestCalculateWaste_MultiDimensional(t *testing.T) {
	calc := NewCalculator(v1.DefaultPricingConfig())

	nodes := []*v1.NodeInfo{
		{
			Name:         "n1",
			InstanceType: "m5.large",
			Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8000},
			Requested:    v1.ResourceQuantities{CPU: 1000, Memory: 4000},
			Used:         v1.ResourceQuantities{CPU: 500, Memory: 3500}, // CPU 50% waste, Mem only 12.5% waste
		},
	}

	waste := calc.CalculateWaste(nodes, nil)
	if waste.CPUWastePct != 50.0 {
		t.Errorf("expected CPU waste 50.0%%, got %.2f%%", waste.CPUWastePct)
	}
	if waste.MemoryWastePct != 12.5 {
		t.Errorf("expected Memory waste 12.5%%, got %.2f%%", waste.MemoryWastePct)
	}
}

func TestCalculateExperimentSavings(t *testing.T) {
	calc := NewCalculator(v1.DefaultPricingConfig())

	// Control: 5 nodes running sparse workloads
	controlNodes := make([]*v1.NodeInfo, 5)
	for i := 0; i < 5; i++ {
		controlNodes[i] = &v1.NodeInfo{
			InstanceType: "m5.large",
			HourlyCost:   0.10,
			Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8000},
			Requested:    v1.ResourceQuantities{CPU: 400, Memory: 1600},
			Used:         v1.ResourceQuantities{CPU: 200, Memory: 800},
		}
	}

	// Treatment: 3 nodes running packed workloads (2 nodes consolidated)
	treatmentNodes := make([]*v1.NodeInfo, 3)
	for i := 0; i < 3; i++ {
		treatmentNodes[i] = &v1.NodeInfo{
			InstanceType: "m5.large",
			HourlyCost:   0.10,
			Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8000},
			Requested:    v1.ResourceQuantities{CPU: 666, Memory: 2666},
			Used:         v1.ResourceQuantities{CPU: 333, Memory: 1333},
		}
	}

	savings := calc.CalculateExperimentSavings(controlNodes, treatmentNodes, 24*time.Hour)

	// Control: 5 * 0.10 * 24 = $12.00
	// Treatment: 3 * 0.10 * 24 = $7.20
	// Savings: $4.80 (40%)
	if savings.ControlCost != 12.00 {
		t.Errorf("expected control cost 12.00, got %.2f", savings.ControlCost)
	}
	if savings.TreatmentCost != 7.20 {
		t.Errorf("expected treatment cost 7.20, got %.2f", savings.TreatmentCost)
	}
	if savings.SavingsPercent != 40.0 {
		t.Errorf("expected savings 40.0%%, got %.2f%%", savings.SavingsPercent)
	}
}
