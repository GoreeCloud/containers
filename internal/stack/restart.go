package stack

import "errors"

// RestartPlan is a source-only dependent-first stop and dependency-first
// startup proposal. No runtime, authorization, health or rollback is verified.
type RestartPlan struct {
	StackName            string  `json:"stackName"`
	SpecSHA256           string  `json:"specSHA256"`
	RequestedService     string  `json:"requestedService"`
	DryRun               bool    `json:"dryRun"`
	DeploymentAuthorized bool    `json:"deploymentAuthorized"`
	LiveStateVerified    bool    `json:"liveStateVerified"`
	RollbackVerified     bool    `json:"rollbackVerified"`
	HealthVerified       bool    `json:"healthVerified"`
	StopStages           []Stage `json:"stopStages"`
	StartStages          []Stage `json:"startStages"`
}

// PlanRestart includes the selected service and all transitive dependents,
// keeping unaffected dependencies outside the restart proposal.
func PlanRestart(spec Spec, service string) (RestartPlan, error) {
	startup, err := DryRun(spec)
	if err != nil {
		return RestartPlan{}, err
	}
	included := map[string]bool{}
	for _, svc := range spec.Services {
		if svc.Name == service {
			included[service] = true
		}
	}
	if !included[service] {
		return RestartPlan{}, errors.New("requested restart service not in declaration")
	}
	for changed := true; changed; {
		changed = false
		for _, svc := range spec.Services {
			if included[svc.Name] {
				continue
			}
			for _, dep := range svc.DependsOn {
				if included[dep] {
					included[svc.Name] = true
					changed = true
					break
				}
			}
		}
	}
	result := RestartPlan{StackName: startup.StackName, SpecSHA256: startup.SpecSHA256, RequestedService: service, DryRun: true, StartStages: []Stage{}, StopStages: []Stage{}}
	for _, stage := range startup.Stages {
		names := []string{}
		for _, name := range stage.IndependentServices {
			if included[name] {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			result.StartStages = append(result.StartStages, Stage{Number: len(result.StartStages), IndependentServices: names})
		}
	}
	for i := len(result.StartStages) - 1; i >= 0; i-- {
		result.StopStages = append(result.StopStages, Stage{Number: len(result.StopStages), IndependentServices: append([]string(nil), result.StartStages[i].IndependentServices...)})
	}
	return result, nil
}
