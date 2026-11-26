package stack

import (
	"testing"
)

func TestImpactDetectsTransitiveDependents(t *testing.T) {
	db := minimalService("db")
	api := minimalService("api")
	ui := minimalService("ui")
	api["dependsOn"] = []string{"db"}
	ui["dependsOn"] = []string{"api"}
	before, err := Parse(fixture(db, api, ui))
	if err != nil {
		t.Fatal(err)
	}
	changedDb := minimalService("db")
	changedDb["replicas"] = 2
	after, err := Parse(fixture(ui, api, changedDb))
	if err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeImpact(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changes) != 1 || report.Changes[0].Service != "db" ||
		len(report.IndirectlyAffectedServices) != 2 ||
		report.IndirectlyAffectedServices[0] != "api" ||
		report.IndirectlyAffectedServices[1] != "ui" || !report.ManualReviewRequired ||
		report.DeploymentAuthorized || report.RollbackVerified || !report.DryRun {
		t.Fatalf("missing transitive risk: %+v", report)
	}
	if report.WorstCaseOverlappingCPUMilli != 1750 ||
		report.WorstCaseOverlappingMemoryBytes != 7*33554432 {
		t.Fatalf("incorrect resource overlap forecast: %+v", report)
	}
}
func TestImpactNoOpRequiresNoReviewButNeverAuthorizes(t *testing.T) {
	left, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	right, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeImpact(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if report.ManualReviewRequired || report.DeploymentAuthorized || report.RollbackVerified ||
		len(report.Changes) > 0 || len(report.IndirectlyAffectedServices) > 0 ||
		report.WorstCaseOverlappingCPUMilli != 0 || report.WorstCaseOverlappingMemoryBytes != 0 {
		t.Fatalf("no-op risk assessment wrong: %+v", report)
	}
}
func TestImpactRejectsUnsafeTransition(t *testing.T) {
	first, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Metadata.Name = "different"
	if _, err := AnalyzeImpact(first, second); err == nil {
		t.Fatal("stack rename accepted")
	}
}
