package stack

import "testing"

func TestCapacityReviewIsAlwaysReadOnly(t *testing.T) {
	s, err := Parse(fixture(minimalService("a")))
	if err != nil {
		t.Fatal(err)
	}
	report, err := AssessResources(s, ResourceBudget{CPUMilli: 250, MemoryBytes: 33554432, MaxInstances: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !report.WithinLimits || report.DeploymentAuthorized || !report.ManualReviewRequired || !report.DryRun {
		t.Fatalf("unsafe capacity result: %+v", report)
	}
	if report.EstimatedCPUMilli != 250 || report.EstimatedMemoryBytes != 33554432 || report.EstimatedInstances != 1 {
		t.Fatalf("resource tally inaccurate: %+v", report)
	}
}
func TestCapacityReviewRejectsOvercommit(t *testing.T) {
	a := minimalService("a")
	a["replicas"] = 3
	s, err := Parse(fixture(a))
	if err != nil {
		t.Fatal(err)
	}
	checks := []ResourceBudget{
		{CPUMilli: 749, MemoryBytes: 33554432 * 3, MaxInstances: 3},
		{CPUMilli: 750, MemoryBytes: 33554432*3 - 1, MaxInstances: 3},
		{CPUMilli: 750, MemoryBytes: 33554432 * 3, MaxInstances: 2},
		{CPUMilli: 1, MemoryBytes: 1, MaxInstances: 1},
	}
	for _, budget := range checks {
		report, err := AssessResources(s, budget)
		if err != nil {
			t.Fatal(err)
		}
		if report.WithinLimits || report.DeploymentAuthorized || len(report.Blockers) == 0 {
			t.Fatalf("overcommit permitted: %+v", report)
		}
	}
}
func TestCapacityReviewRequiresValidExplicitLimits(t *testing.T) {
	s, err := Parse(fixture(minimalService("a")))
	if err != nil {
		t.Fatal(err)
	}
	for _, budget := range []ResourceBudget{
		{}, {CPUMilli: 250, MemoryBytes: 33554432, MaxInstances: 0},
		{CPUMilli: -1, MemoryBytes: 33554432, MaxInstances: 1},
		{CPUMilli: 250, MemoryBytes: -1, MaxInstances: 1},
		{CPUMilli: 250, MemoryBytes: 33554432, MaxInstances: 4097},
	} {
		if _, err := AssessResources(s, budget); err == nil {
			t.Errorf("invalid limits accepted %+v", budget)
		}
	}
}
