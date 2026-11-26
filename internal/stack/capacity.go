package stack

import "errors"

// ResourceBudget is a caller-supplied synthetic or audited host capacity
// envelope. It is NOT automatic host detection or permission to deploy.
type ResourceBudget struct {
	CPUMilli     int64 `json:"cpuMilli"`
	MemoryBytes  int64 `json:"memoryBytes"`
	MaxInstances int   `json:"maxInstances"`
}
type CapacityReview struct {
	StackName            string         `json:"stackName"`
	SpecSHA256           string         `json:"specSHA256"`
	DryRun               bool           `json:"dryRun"`
	WithinLimits         bool           `json:"withinLimits"`
	DeploymentAuthorized bool           `json:"deploymentAuthorized"`
	ManualReviewRequired bool           `json:"manualReviewRequired"`
	EstimatedCPUMilli    int64          `json:"estimatedCpuMilli"`
	EstimatedMemoryBytes int64          `json:"estimatedMemoryBytes"`
	EstimatedInstances   int            `json:"estimatedInstances"`
	Budget               ResourceBudget `json:"budget"`
	Blockers             []string       `json:"blockers"`
}

// AssessResources is deterministic, read-only, and independent of Docker,
// Podman, the kernel and services. It never assumes that a declared budget
// equals currently free system resources, and NEVER authorizes execution.
func AssessResources(s Spec, budget ResourceBudget) (CapacityReview, error) {
	var empty CapacityReview
	if err := Validate(s); err != nil {
		return empty, err
	}
	if budget.CPUMilli <= 0 || budget.MemoryBytes <= 0 ||
		budget.MaxInstances <= 0 || budget.MaxInstances > 4096 {
		return empty, errors.New("explicit positive finite CPU, memory and instance budgets required")
	}
	plan, err := DryRun(s)
	if err != nil {
		return empty, err
	}
	result := CapacityReview{
		StackName: s.Metadata.Name, SpecSHA256: plan.SpecSHA256, DryRun: true,
		WithinLimits: true, DeploymentAuthorized: false, ManualReviewRequired: true,
		EstimatedCPUMilli:    plan.TotalCPUMilli,
		EstimatedMemoryBytes: plan.TotalMemoryBytes, Budget: budget,
		Blockers: make([]string, 0),
	}
	for _, svc := range s.Services {
		result.EstimatedInstances += svc.Replicas
	}
	if result.EstimatedCPUMilli > budget.CPUMilli {
		result.WithinLimits = false
		result.Blockers = append(result.Blockers, "declared CPU demand exceeds supplied budget")
	}
	if result.EstimatedMemoryBytes > budget.MemoryBytes {
		result.WithinLimits = false
		result.Blockers = append(result.Blockers, "declared memory demand exceeds supplied budget")
	}
	if result.EstimatedInstances > budget.MaxInstances {
		result.WithinLimits = false
		result.Blockers = append(result.Blockers, "declared instances exceed supplied budget")
	}
	// This result intentionally excludes identities and private secret refs.
	return result, nil
}
