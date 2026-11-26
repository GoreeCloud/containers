package stack

import (
	"encoding/json"
	"github.com/GoreeCloud/containers/internal/engine"
	"strings"
	"testing"
)

func TestObserveInstancesNameCorrelationOnly(t *testing.T) {
	db := minimalService("db")
	api := minimalService("api")
	api["dependsOn"] = []string{"db"}
	api["replicas"] = 2
	api["secretRefs"] = []string{"private-reference"}
	s, e := Parse(fixture(db, api))
	if e != nil {
		t.Fatal(e)
	}
	p, e := PlanInstances(s)
	if e != nil {
		t.Fatal(e)
	}
	first := p.StartWaves[0].Instances[0].ID
	second := p.StartWaves[1].Instances[0].ID
	i := engine.InventoryReport{ReadOnly: true, Running: 1, Uncertain: 1, RecoveryRequired: true, Entries: []engine.InventoryEntry{{ID: first, MetadataPresent: true, Status: "running"}, {ID: second, MetadataPresent: true, Status: "stopped", PendingOperation: "start"}, {ID: "private-unrelated", Status: "stopped", MetadataPresent: true}}}
	i.Stopped = 1
	r, e := ObserveInstances(s, p.PlanSHA256, i)
	if e != nil || r.Expected != 3 || r.Running != 1 || r.Missing != 1 || r.Uncertain != 1 || r.UnmatchedRuntimeCount != 1 || !r.RecoveryRequired || r.InstanceIdentityVerified || r.HealthVerified || r.DeploymentAuthorized {
		t.Fatal(r, e)
	}
	if r.Instances[1].DependencyRunningCount != 1 || r.Instances[1].Review != "recovery_review_required" {
		t.Fatal(r)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "private-unrelated") || strings.Contains(string(b), "private-reference") {
		t.Fatal("unrelated or secret data leaked")
	}
	if _, e = ObserveInstances(s, "sha256:"+strings.Repeat("0", 64), i); e == nil {
		t.Fatal("stale pin accepted")
	}
	i.Entries = append(i.Entries, i.Entries[0])
	if _, e = ObserveInstances(s, p.PlanSHA256, i); e == nil {
		t.Fatal("duplicate accepted")
	}
}
