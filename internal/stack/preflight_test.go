package stack

import (
	"strings"
	"testing"
)

func TestPreflightCombinesSameSourceRevisionBudgetAndImpact(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	changed := minimalService("web")
	changed["replicas"] = 2
	next, err := Parse(fixture(changed))
	if err != nil {
		t.Fatal(err)
	}
	fp, err := DryRun(old)
	if err != nil {
		t.Fatal(err)
	}
	ample := ResourceBudget{CPUMilli: 2000, MemoryBytes: 1 << 30, MaxInstances: 8}
	ok, err := ReviewPreflight(old, next, fp.SpecSHA256, ample)
	if err != nil {
		t.Fatal(err)
	}
	if !ok.DryRun || !ok.SourceAndBudgetChecksPassed || !ok.Revision.MatchesExpected ||
		!ok.Capacity.WithinBudget || len(ok.Impact.Changes) != 1 ||
		ok.DeploymentAuthorized || ok.LiveStateVerified || ok.RollbackVerified ||
		!ok.ManualReviewRequired || len(ok.Blockers) != 0 {
		t.Fatalf("budget and revision match misrepresented approval: %+v", ok)
	}
	stale := "sha256:" + strings.Repeat("d", 64)
	rejected, err := ReviewPreflight(old, next, stale, ample)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.SourceAndBudgetChecksPassed || len(rejected.Blockers) != 1 || !rejected.ManualReviewRequired {
		t.Fatalf("stale expected revision was accepted: %+v", rejected)
	}
	tiny := ResourceBudget{CPUMilli: 200, MemoryBytes: 16 << 20, MaxInstances: 1}
	oversubscribed, err := ReviewPreflight(old, next, fp.SpecSHA256, tiny)
	if err != nil {
		t.Fatal(err)
	}
	if oversubscribed.SourceAndBudgetChecksPassed || len(oversubscribed.Blockers) != 3 {
		t.Fatalf("resource overcommit not propagated: %+v", oversubscribed)
	}
}
func TestPreflightRejectsInvalidParametersAndNoOpNeverDeploys(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := DryRun(old)
	if err != nil {
		t.Fatal(err)
	}
	cap := ResourceBudget{CPUMilli: 400, MemoryBytes: 64 << 20, MaxInstances: 4}
	same, err := ReviewPreflight(old, old, plan.SpecSHA256, cap)
	if err != nil || !same.SourceAndBudgetChecksPassed || same.ManualReviewRequired ||
		same.DeploymentAuthorized || same.LiveStateVerified || same.RollbackVerified {
		t.Fatalf("no-op claimed approval: %+v %v", same, err)
	}
	if _, err := ReviewPreflight(old, old, "", cap); err == nil {
		t.Fatal("missing expected revision accepted")
	}
	if _, err := ReviewPreflight(old, old, plan.SpecSHA256, ResourceBudget{}); err == nil {
		t.Fatal("missing budget accepted")
	}
}
