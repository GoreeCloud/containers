package stack

import (
	"strings"
	"testing"
)

func TestRevisionGuardDetectsStaleSourceDeclaration(t *testing.T) {
	current, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	changed := minimalService("web")
	changed["replicas"] = 2
	future, err := Parse(fixture(changed))
	if err != nil {
		t.Fatal(err)
	}
	before, err := DryRun(current)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := ReviewRevision(current, future, before.SpecSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !valid.MatchesExpected || valid.LiveStateVerified || valid.DeploymentAuthorized ||
		valid.RollbackVerified || !valid.ManualReviewRequired || valid.ChangeCount != 1 {
		t.Fatalf("matching caller-declared revision treated as deployment approval: %+v", valid)
	}
	other := "sha256:" + strings.Repeat("b", 64)
	stale, err := ReviewRevision(current, future, other)
	if err != nil {
		t.Fatal(err)
	}
	if stale.MatchesExpected || !stale.ManualReviewRequired ||
		len(stale.Blockers) < 2 || stale.DeploymentAuthorized {
		t.Fatalf("stale revision accepted: %+v", stale)
	}
}
func TestRevisionGuardRejectsMalformedOrRenamedStacks(t *testing.T) {
	current, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := DryRun(current)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "sha256:not-a-hash", strings.Repeat("a", 64), "sha256:" + strings.Repeat("A", 64)} {
		if _, err := ReviewRevision(current, current, bad); err == nil {
			t.Fatalf("bad expected digest %q accepted", bad)
		}
	}
	other := current
	other.Metadata.Name = "other"
	if _, err := ReviewRevision(current, other, plan.SpecSHA256); err == nil {
		t.Fatal("implicit stack rename allowed")
	}
}
func TestRevisionNoOpStillCannotAttestLiveState(t *testing.T) {
	current, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := DryRun(current)
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewRevision(current, current, plan.SpecSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !review.MatchesExpected || review.ChangeCount != 0 || review.ManualReviewRequired ||
		review.LiveStateVerified || review.DeploymentAuthorized || review.RollbackVerified {
		t.Fatalf("source fingerprint misrepresented live state: %+v", review)
	}
}
