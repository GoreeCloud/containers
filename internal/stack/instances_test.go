package stack

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInstancePlanReplicaDependencyWaves(t *testing.T) {
	db := minimalService("db")
	db["replicas"] = 2
	api := minimalService("api")
	api["replicas"] = 3
	api["dependsOn"] = []string{"db"}
	api["secretRefs"] = []string{"private-ref"}
	cache := minimalService("cache")
	s, e := Parse(fixture(api, db, cache))
	if e != nil {
		t.Fatal(e)
	}
	p, e := PlanInstances(s)
	if e != nil {
		t.Fatal(e)
	}
	if p.InstanceCount != 6 || len(p.StartWaves) != 2 || len(p.StartWaves[0].Instances) != 3 || len(p.StartWaves[1].Instances) != 3 || p.DeploymentAuthorized || p.HealthVerified || p.MIPSAuthorizationVerified {
		t.Fatal(p)
	}
	for _, i := range p.StartWaves[1].Instances {
		if len(i.Requires) != 2 || i.Service != "api" {
			t.Fatal(i)
		}
	}
	if len(p.StopWaves) != 2 || p.StopWaves[0][0] != p.StartWaves[1].Instances[0].ID {
		t.Fatal("reverse order wrong")
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "private-ref") {
		t.Fatal("secret reference leaked")
	}
	reordered, e := Parse(fixture(cache, db, api))
	if e != nil {
		t.Fatal(e)
	}
	q, e := PlanInstances(reordered)
	if e != nil || q.PlanSHA256 != p.PlanSHA256 {
		t.Fatal(q, e)
	}
	if _, e = ReviewInstancePlan(s, p.PlanSHA256); e != nil {
		t.Fatal(e)
	}
	s.Services[0].Replicas = 4
	if _, e = ReviewInstancePlan(s, p.PlanSHA256); e == nil {
		t.Fatal("stale plan accepted")
	}
}
func TestInstanceIDsSeparateAmbiguousNames(t *testing.T) {
	if instanceID("a-b", "c", 1) == instanceID("a", "b-c", 1) || instanceID("a", "b", 1) == instanceID("a", "b", 2) {
		t.Fatal("ambiguous IDs")
	}
}
