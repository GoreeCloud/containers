package stack

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSensitiveSecretReferenceChangesRequireThreeDistinctPolicyReviews(t *testing.T) {
	source, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := DryRun(source)
	if err != nil {
		t.Fatal(err)
	}
	next := source
	next.Services = append([]Service(nil), source.Services...)
	next.Services[0].SecretRefs = []string{"private-vaultkey"}
	report, err := ReviewPreflight(source, next, plan.SpecSHA256, ResourceBudget{
		CPUMilli: 8000, MemoryBytes: 1 << 30, MaxInstances: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.SourceAndBudgetChecksPassed || !report.SensitivePolicyReviewRequired ||
		report.SecretReferenceChangeCount != 1 || !report.GoreeCloudIdentityReviewRequired ||
		!report.GoreeShieldReviewRequired || !report.WardveilReviewRequired ||
		report.MIPSAuthorizationVerified || !report.ManualReviewRequired ||
		report.DeploymentAuthorized || len(report.Blockers) != 1 {
		t.Fatalf("sensitive transition not fail-closed: %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("private-vaultkey")) {
		t.Fatal("private secret reference name leaked in review output")
	}
	twice := next
	twice.Services = append([]Service(nil), next.Services...)
	twice.Services[0].SecretRefs = []string{"z-vault", "a-vault"}
	again := twice
	again.Services = append([]Service(nil), twice.Services...)
	again.Services[0].SecretRefs = []string{"a-vault", "z-vault"}
	if countChangedSecretReferenceServices(twice, again) != 0 {
		t.Fatal("secret list permutation incorrectly triggers review")
	}
}
func TestSecretReferenceAdditionRemovalReplacementRequirePolicyReview(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	newer := old
	newer.Services = append([]Service(nil), old.Services...)
	newer.Services[0].SecretRefs = []string{"my-vault"}
	if countChangedSecretReferenceServices(old, newer) != 1 ||
		countChangedSecretReferenceServices(newer, old) != 1 {
		t.Fatal("secret reference addition/removal not detected")
	}
	changed := newer
	changed.Services = append([]Service(nil), newer.Services...)
	changed.Services[0].SecretRefs = []string{"new-vault"}
	if countChangedSecretReferenceServices(newer, changed) != 1 {
		t.Fatal("secret reference replacement not detected")
	}
}
