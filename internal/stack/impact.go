package stack

import (
	"sort"
)

// ImpactReport is a conservative proposed transition blast-radius estimate.
// It does NOT resolve real running tasks, validate health, enforce resources,
// or authorize any deployment.
type ImpactReport struct {
	StackName                       string       `json:"stackName"`
	FromSpecSHA256                  string       `json:"fromSpecSHA256"`
	ToSpecSHA256                    string       `json:"toSpecSHA256"`
	DryRun                          bool         `json:"dryRun"`
	DeploymentAuthorized            bool         `json:"deploymentAuthorized"`
	RollbackVerified                bool         `json:"rollbackVerified"`
	Changes                         []ChangeStep `json:"changes"`
	IndirectlyAffectedServices      []string     `json:"indirectlyAffectedServices"`
	WorstCaseOverlappingCPUMilli    int64        `json:"worstCaseOverlappingCpuMilli"`
	WorstCaseOverlappingMemoryBytes int64        `json:"worstCaseOverlappingMemoryBytes"`
	ManualReviewRequired            bool         `json:"manualReviewRequired"`
}

// AnalyzeImpact considers both old and new dependency graphs. An unchanged
// dependent can still be affected by its changed upstream service. Resource
// overlap is a deliberately pessimistic worst-case old+new total (not a
// prediction of actual reservation), and must be approved by a future
// authorized, resource-aware native executor.
func AnalyzeImpact(before, after Spec) (ImpactReport, error) {
	changes, err := Diff(before, after)
	if err != nil {
		return ImpactReport{}, err
	}
	current, err := DryRun(before)
	if err != nil {
		return ImpactReport{}, err
	}
	future, err := DryRun(after)
	if err != nil {
		return ImpactReport{}, err
	}
	report := ImpactReport{
		StackName: changes.StackName, FromSpecSHA256: changes.FromSpecSHA256,
		ToSpecSHA256: changes.ToSpecSHA256, DryRun: true,
		DeploymentAuthorized: false, RollbackVerified: false,
		Changes: changes.Changes, IndirectlyAffectedServices: make([]string, 0),
		ManualReviewRequired: changes.ManualReviewRequired,
	}
	if len(changes.Changes) == 0 {
		return report, nil
	}
	report.WorstCaseOverlappingCPUMilli = current.TotalCPUMilli + future.TotalCPUMilli
	report.WorstCaseOverlappingMemoryBytes = current.TotalMemoryBytes + future.TotalMemoryBytes
	touched := map[string]bool{}
	for _, c := range changes.Changes {
		touched[c.Service] = true
	}
	reverse := map[string][]string{}
	for _, spec := range []Spec{before, after} {
		for _, svc := range spec.Services {
			for _, dep := range svc.DependsOn {
				reverse[dep] = append(reverse[dep], svc.Name)
			}
		}
	}
	discovered := map[string]bool{}
	queue := make([]string, 0, len(touched))
	for name := range touched {
		queue = append(queue, name)
		discovered[name] = true
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		top := queue[0]
		queue = queue[1:]
		for _, child := range reverse[top] {
			if discovered[child] {
				continue
			}
			discovered[child] = true
			queue = append(queue, child)
			if !touched[child] {
				report.IndirectlyAffectedServices = append(report.IndirectlyAffectedServices, child)
			}
		}
	}
	sort.Strings(report.IndirectlyAffectedServices)
	return report, nil
}
