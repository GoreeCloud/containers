package stack

// ShutdownPlan is an advisory reverse-dependency plan. It neither contacts
// a runtime nor implies a live workload exists or is authorized to stop.
type ShutdownStep struct {
	Service   string `json:"service"`
	Replicas  int    `json:"replicas"`
	Stage     int    `json:"stage"`
	Operation string `json:"operation"`
}
type ShutdownPlan struct {
	StackName            string         `json:"stackName"`
	SpecSHA256           string         `json:"specSHA256"`
	DryRun               bool           `json:"dryRun"`
	LiveStateVerified    bool           `json:"liveStateVerified"`
	DeploymentAuthorized bool           `json:"deploymentAuthorized"`
	RollbackVerified     bool           `json:"rollbackVerified"`
	Stages               []Stage        `json:"stages"`
	Steps                []ShutdownStep `json:"steps"`
}

// PlanShutdown reverses the validated startup layers. Independent services
// can be reviewed together; every dependent precedes its dependencies.
func PlanShutdown(spec Spec) (ShutdownPlan, error) {
	start, err := DryRun(spec)
	if err != nil {
		return ShutdownPlan{}, err
	}
	result := ShutdownPlan{StackName: start.StackName, SpecSHA256: start.SpecSHA256, DryRun: true, Stages: make([]Stage, 0, len(start.Stages)), Steps: make([]ShutdownStep, 0, len(start.Steps))}
	for i := len(start.Stages) - 1; i >= 0; i-- {
		stage := start.Stages[i]
		n := len(result.Stages)
		result.Stages = append(result.Stages, Stage{Number: n, IndependentServices: append([]string(nil), stage.IndependentServices...)})
		for _, step := range start.Steps {
			if step.Stage == i {
				// No image or secret identifier is needed for a stop plan.
				result.Steps = append(result.Steps, ShutdownStep{Service: step.Service, Replicas: step.Replicas, Stage: n, Operation: "would-stop"})
			}
		}
	}
	return result, nil
}
