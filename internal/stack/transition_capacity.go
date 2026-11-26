package stack

import "errors"

// TransitionCapacityReview is a conservative, user-supplied budget comparison
// for overlapping old and new native stack declarations. It is never a live
// system admission, scheduler, health check, or permission to deploy.
type TransitionCapacityReview struct {
	StackName                  string         `json:"stackName"`
	FromSpecSHA256             string         `json:"fromSpecSHA256"`
	ToSpecSHA256               string         `json:"toSpecSHA256"`
	DryRun                     bool           `json:"dryRun"`
	WithinBudget               bool           `json:"withinBudget"`
	DeploymentAuthorized       bool           `json:"deploymentAuthorized"`
	RollbackVerified           bool           `json:"rollbackVerified"`
	ManualReviewRequired       bool           `json:"manualReviewRequired"`
	EstimatedPeakCPUMilli      int64          `json:"estimatedPeakCpuMilli"`
	EstimatedPeakMemoryBytes   int64          `json:"estimatedPeakMemoryBytes"`
	EstimatedPeakInstances     int            `json:"estimatedPeakInstances"`
	Budget                     ResourceBudget `json:"budget"`
	IndirectlyAffectedServices []string       `json:"indirectlyAffectedServices"`
	Blockers                   []string       `json:"blockers"`
}

// AssessTransitionCapacity conservatively assumes old and new deployments
// may coexist while changing. This deliberately overestimates changes; the
// actual executor must prove the intended rollout strategy and cgroup quotas.
func AssessTransitionCapacity(before, after Spec, budget ResourceBudget) (TransitionCapacityReview, error) {
	var empty TransitionCapacityReview
	if budget.CPUMilli <= 0 || budget.MemoryBytes <= 0 || budget.MaxInstances <= 0 ||
		budget.MaxInstances > 4096 {
		return empty, errors.New("positive finite explicit transition capacity required")
	}
	impact, err := AnalyzeImpact(before, after)
	if err != nil {
		return empty, err
	}
	oldPlan, err := DryRun(before)
	if err != nil {
		return empty, err
	}
	newPlan, err := DryRun(after)
	if err != nil {
		return empty, err
	}
	oldInstances := 0
	newInstances := 0
	for _, svc := range before.Services {
		oldInstances += svc.Replicas
	}
	for _, svc := range after.Services {
		newInstances += svc.Replicas
	}
	report := TransitionCapacityReview{
		StackName: impact.StackName, FromSpecSHA256: impact.FromSpecSHA256,
		ToSpecSHA256: impact.ToSpecSHA256, DryRun: true, WithinBudget: true,
		DeploymentAuthorized: false, RollbackVerified: false,
		ManualReviewRequired:       impact.ManualReviewRequired,
		Budget:                     budget,
		IndirectlyAffectedServices: impact.IndirectlyAffectedServices,
		Blockers:                   make([]string, 0),
	}
	if len(impact.Changes) > 0 {
		report.EstimatedPeakCPUMilli = oldPlan.TotalCPUMilli + newPlan.TotalCPUMilli
		report.EstimatedPeakMemoryBytes = oldPlan.TotalMemoryBytes + newPlan.TotalMemoryBytes
		report.EstimatedPeakInstances = oldInstances + newInstances
	} else {
		report.EstimatedPeakCPUMilli = oldPlan.TotalCPUMilli
		report.EstimatedPeakMemoryBytes = oldPlan.TotalMemoryBytes
		report.EstimatedPeakInstances = oldInstances
	}
	if report.EstimatedPeakCPUMilli > budget.CPUMilli {
		report.WithinBudget = false
		report.Blockers = append(report.Blockers, "potential old-plus-new CPU overlap exceeds supplied budget")
	}
	if report.EstimatedPeakMemoryBytes > budget.MemoryBytes {
		report.WithinBudget = false
		report.Blockers = append(report.Blockers, "potential old-plus-new memory overlap exceeds supplied budget")
	}
	if report.EstimatedPeakInstances > budget.MaxInstances {
		report.WithinBudget = false
		report.Blockers = append(report.Blockers, "potential old-plus-new instances exceed supplied budget")
	}
	return report, nil
}
