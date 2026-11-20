package stack

import (
	"testing"
)

func TestDiffNoChangeDespiteServiceReorder(t *testing.T) {
	api := minimalService("api")
	db := minimalService("db")
	api["dependsOn"] = []string{"db"}
	before, err := Parse(fixture(api, db))
	if err != nil {
		t.Fatal(err)
	}
	after, err := Parse(fixture(db, api))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Diff(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || plan.FromSpecSHA256 != plan.ToSpecSHA256 || len(plan.Changes) != 0 || plan.ManualReviewRequired {
		t.Fatalf("unexpected no-op diff %+v", plan)
	}
}
func TestDiffDependencySafeAddRemove(t *testing.T) {
	oldDb := minimalService("db")
	oldApi := minimalService("api")
	oldApi["dependsOn"] = []string{"db"}
	newStore := minimalService("store")
	newUI := minimalService("ui")
	newUI["dependsOn"] = []string{"store"}
	before, err := Parse(fixture(oldApi, oldDb))
	if err != nil {
		t.Fatal(err)
	}
	after, err := Parse(fixture(newUI, newStore))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Diff(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 4 || !plan.ManualReviewRequired || !plan.DryRun {
		t.Fatalf("unexpected plan %+v", plan)
	}
	names := []string{"store", "ui", "api", "db"}
	kinds := []ChangeKind{ChangeAdd, ChangeAdd, ChangeRemove, ChangeRemove}
	for i, step := range plan.Changes {
		if step.Service != names[i] || step.Action != kinds[i] || !step.ReviewRequired {
			t.Fatalf("unsafe ordering or review omission: %+v", plan.Changes)
		}
	}
}
func TestDiffRejectsRenameAndIndicatesReplacements(t *testing.T) {
	a := minimalService("web")
	old, err := Parse(fixture(a))
	if err != nil {
		t.Fatal(err)
	}
	a["replicas"] = 2
	updated, err := Parse(fixture(a))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Diff(old, updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Action != ChangeReplace || !plan.ManualReviewRequired || plan.FromSpecSHA256 == plan.ToSpecSHA256 {
		t.Fatalf("replacement not guarded: %+v", plan)
	}
	updated.Metadata.Name = "new-stack"
	if _, err := Diff(old, updated); err == nil {
		t.Fatal("implicit stack rename accepted")
	}
}
