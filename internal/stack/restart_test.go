package stack

import (
	"reflect"
	"testing"
)

func TestRestartPlanIncludesTransitiveDependentsOnly(t *testing.T) {
	db, api, worker, web, other := minimalService("db"), minimalService("api"), minimalService("worker"), minimalService("web"), minimalService("other")
	api["dependsOn"] = []string{"db"}
	worker["dependsOn"] = []string{"api"}
	web["dependsOn"] = []string{"api"}
	spec, err := Parse(fixture(db, api, worker, web, other))
	if err != nil {
		t.Fatal(err)
	}
	p, err := PlanRestart(spec, "api")
	if err != nil {
		t.Fatal(err)
	}
	if !p.DryRun || p.DeploymentAuthorized || p.LiveStateVerified || p.RollbackVerified || p.HealthVerified {
		t.Fatal("false acceptance")
	}
	wantStart := []Stage{{Number: 0, IndependentServices: []string{"api"}}, {Number: 1, IndependentServices: []string{"web", "worker"}}}
	wantStop := []Stage{{Number: 0, IndependentServices: []string{"web", "worker"}}, {Number: 1, IndependentServices: []string{"api"}}}
	if !reflect.DeepEqual(p.StartStages, wantStart) || !reflect.DeepEqual(p.StopStages, wantStop) {
		t.Fatalf("wrong ordering: %+v", p)
	}
	if _, err := PlanRestart(spec, "absent"); err == nil {
		t.Fatal("unknown target accepted")
	}
	spec.Services[0], spec.Services[4] = spec.Services[4], spec.Services[0]
	again, err := PlanRestart(spec, "api")
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatal("nondeterministic restart plan")
	}
}
