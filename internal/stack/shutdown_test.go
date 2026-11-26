package stack

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestShutdownPlanReverseDependencies(t *testing.T) {
	a, b, c, d := minimalService("a"), minimalService("b"), minimalService("c"), minimalService("d")
	b["dependsOn"] = []string{"a"}
	c["dependsOn"] = []string{"a"}
	d["dependsOn"] = []string{"b", "c"}
	spec, err := Parse(fixture(a, b, c, d))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanShutdown(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || plan.LiveStateVerified || plan.DeploymentAuthorized || plan.RollbackVerified {
		t.Fatal("false authorization")
	}
	expected := [][]string{{"d"}, {"b", "c"}, {"a"}}
	for i, stage := range plan.Stages {
		if stage.Number != i || !reflect.DeepEqual(stage.IndependentServices, expected[i]) {
			t.Fatalf("stage %v", stage)
		}
	}
	raw, _ := json.Marshal(plan)
	if strings.Contains(string(raw), a["image"].(string)) {
		t.Fatal("image identifier disclosed")
	}
	spec.Services[0], spec.Services[3] = spec.Services[3], spec.Services[0]
	reordered, err := PlanShutdown(spec)
	if err != nil || !reflect.DeepEqual(plan, reordered) {
		t.Fatalf("order-dependent result: %v", err)
	}
}
