package stack

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestImageReferenceChangeRequiresWardveilAndGoreeShieldReview(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := DryRun(old)
	if err != nil {
		t.Fatal(err)
	}
	service := minimalService("web")
	service["image"] = "sha256:" + strings.Repeat("f", 64)
	next, err := Parse(fixture(service))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ReviewPreflight(old, next, plan.SpecSHA256, ResourceBudget{
		CPUMilli: 9000, MemoryBytes: 1 << 30, MaxInstances: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !r.SourceAndBudgetChecksPassed || r.ImageReferenceChangeCount != 1 ||
		r.ImageProvenanceVerified || !r.SensitivePolicyReviewRequired ||
		!r.GoreeShieldReviewRequired || !r.WardveilReviewRequired ||
		r.GoreeCloudIdentityReviewRequired || r.MIPSAuthorizationVerified ||
		r.DeploymentAuthorized || !r.ManualReviewRequired {
		t.Fatalf("image provenance change incorrectly approved: %+v", r)
	}
	output, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(output, []byte(strings.Repeat("f", 64))) {
		t.Fatal("image identifier leaked into policy-only report")
	}
	if countChangedImageReferenceServices(old, old) != 0 {
		t.Fatal("no-op wrongly requires new image")
	}
	if countChangedImageReferenceServices(next, old) != 1 {
		t.Fatal("image revert omitted")
	}
}
func TestImageServiceAdditionAndRemovalRequireReview(t *testing.T) {
	old, err := Parse(fixture(minimalService("web")))
	if err != nil {
		t.Fatal(err)
	}
	next := old
	next.Services = append([]Service(nil), old.Services...)
	db := old.Services[0]
	db.Name = "db"
	next.Services = append(next.Services, db)
	if countChangedImageReferenceServices(old, next) != 1 ||
		countChangedImageReferenceServices(next, old) != 1 {
		t.Fatal("service image addition/removal skipped")
	}
}
