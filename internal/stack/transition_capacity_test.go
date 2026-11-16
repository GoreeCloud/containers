package stack

import "testing"

func TestTransitionAdmissionRejectsOverlap(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	newVersion := minimalService("web")
	newVersion["replicas"] = 2
	next, err := Parse(fixture(newVersion))
	if err != nil {
		t.Fatal(err)
	}
	budget := ResourceBudget{CPUMilli: 500, MemoryBytes: 67108864, MaxInstances: 2}
	report, err := AssessTransitionCapacity(old, next, budget)
	if err != nil {
		t.Fatal(err)
	}
	if report.WithinBudget || report.DeploymentAuthorized || report.RollbackVerified ||
		!report.ManualReviewRequired || report.EstimatedPeakCPUMilli != 750 ||
		report.EstimatedPeakMemoryBytes != 100663296 || report.EstimatedPeakInstances != 3 ||
		len(report.Blockers) != 3 {
		t.Fatalf("unsafe overlap accepted %+v", report)
	}
}
func TestTransitionAdmissionNoOpChecksCurrentRequirements(t *testing.T) {
	s, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	budget := ResourceBudget{CPUMilli: 249, MemoryBytes: 33554432, MaxInstances: 1}
	report, err := AssessTransitionCapacity(s, s, budget)
	if err != nil {
		t.Fatal(err)
	}
	if report.WithinBudget || report.ManualReviewRequired || report.DeploymentAuthorized ||
		report.EstimatedPeakCPUMilli != 250 || report.EstimatedPeakInstances != 1 {
		t.Fatalf("no-op existing capacity skipped %+v", report)
	}
}
func TestTransitionAdmissionNeverApprovesEvenIfInBudget(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	newer := minimalService("web")
	newer["replicas"] = 2
	next, err := Parse(fixture(newer))
	if err != nil {
		t.Fatal(err)
	}
	report, err := AssessTransitionCapacity(old, next, ResourceBudget{CPUMilli: 2000, MemoryBytes: 1 << 30, MaxInstances: 16})
	if err != nil {
		t.Fatal(err)
	}
	if !report.WithinBudget || report.DeploymentAuthorized || report.RollbackVerified ||
		!report.ManualReviewRequired || len(report.Blockers) > 0 {
		t.Fatalf("budget was treated as authorization %+v", report)
	}
}
func TestTransitionAdmissionRequiresValidBudgetAndStack(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	for _, budget := range []ResourceBudget{{}, {CPUMilli: 1, MemoryBytes: 1, MaxInstances: 0}, {CPUMilli: 1, MemoryBytes: -1, MaxInstances: 1}} {
		if _, err := AssessTransitionCapacity(old, old, budget); err == nil {
			t.Fatal("invalid budget accepted")
		}
	}
	changed := old
	changed.Metadata.Name = "other"
	if _, err := AssessTransitionCapacity(old, changed, ResourceBudget{CPUMilli: 500, MemoryBytes: 67108864, MaxInstances: 2}); err == nil {
		t.Fatal("stack rename incorrectly admitted")
	}
}
