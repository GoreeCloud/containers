package stack

import (
	"errors"
)

// PreflightReport composes independent source-document guards over the SAME
// parsed old/new declarations. It does not inspect actual deployed workloads
// and never authorizes their creation, update or removal.
type PreflightReport struct {
	DryRun                           bool                     `json:"dryRun"`
	SourceAndBudgetChecksPassed      bool                     `json:"sourceAndBudgetChecksPassed"`
	DeploymentAuthorized             bool                     `json:"deploymentAuthorized"`
	LiveStateVerified                bool                     `json:"liveStateVerified"`
	RollbackVerified                 bool                     `json:"rollbackVerified"`
	ManualReviewRequired             bool                     `json:"manualReviewRequired"`
	SensitivePolicyReviewRequired    bool                     `json:"sensitivePolicyReviewRequired"`
	SecretReferenceChangeCount       int                      `json:"secretReferenceChangeCount"`
	ImageReferenceChangeCount        int                      `json:"imageReferenceChangeCount"`
	ImageProvenanceVerified          bool                     `json:"imageProvenanceVerified"`
	GoreeCloudIdentityReviewRequired bool                     `json:"goreeCloudIdentityReviewRequired"`
	GoreeShieldReviewRequired        bool                     `json:"goreeShieldReviewRequired"`
	WardveilReviewRequired           bool                     `json:"wardveilReviewRequired"`
	MIPSAuthorizationVerified        bool                     `json:"mipsAuthorizationVerified"`
	Revision                         RevisionReview           `json:"revision"`
	Capacity                         TransitionCapacityReview `json:"capacity"`
	Impact                           ImpactReport             `json:"impact"`
	Blockers                         []string                 `json:"blockers"`
}

// ReviewPreflight reads each document as a stable in-memory value, then
// performs revision, declared capacity and dependency impact checks over the
// same values. A future executor must separately attest to the running
// generation under an authenticated lock and re-evaluate all its controls.
func ReviewPreflight(before, after Spec, expected string, budget ResourceBudget) (PreflightReport, error) {
	var empty PreflightReport
	if expected == "" {
		return empty, errors.New("expected prior source revision required")
	}
	revision, err := ReviewRevision(before, after, expected)
	if err != nil {
		return empty, err
	}
	capacity, err := AssessTransitionCapacity(before, after, budget)
	if err != nil {
		return empty, err
	}
	impact, err := AnalyzeImpact(before, after)
	if err != nil {
		return empty, err
	}
	// The impact planner can expose exact image digests for an authorized
	// operator, but preflight is a policy-facing diagnostic and must not
	// expose caller-supplied image identifiers. Keep service names and action.
	for i := range impact.Changes {
		impact.Changes[i].ImageDigest = ""
	}
	result := PreflightReport{
		DryRun: true, Revision: revision, Capacity: capacity, Impact: impact,
		SourceAndBudgetChecksPassed: revision.MatchesExpected && capacity.WithinBudget,
		DeploymentAuthorized:        false, LiveStateVerified: false, RollbackVerified: false,
		ManualReviewRequired: revision.ManualReviewRequired || capacity.ManualReviewRequired,
		Blockers:             make([]string, 0, 2+len(capacity.Blockers)),
	}
	if !revision.MatchesExpected {
		result.ManualReviewRequired = true
		result.Blockers = append(result.Blockers, "expected source revision does not match prior declaration")
	}
	result.Blockers = append(result.Blockers, capacity.Blockers...)
	// Names of external secrets are never read from a provider.
	// Do not resolve, log or expose any reference identifiers here.
	secretChanges := countChangedSecretReferenceServices(before, after)
	result.SecretReferenceChangeCount = secretChanges
	if secretChanges > 0 {
		result.SensitivePolicyReviewRequired = true
		result.GoreeCloudIdentityReviewRequired = true
		result.GoreeShieldReviewRequired = true
		result.WardveilReviewRequired = true
		result.ManualReviewRequired = true
		result.Blockers = append(result.Blockers, "secret reference changes require GoreeCloud Identity, GoreeShield and Wardveil review")
	}
	imageChanges := countChangedImageReferenceServices(before, after)
	result.ImageReferenceChangeCount = imageChanges
	result.ImageProvenanceVerified = false
	if imageChanges > 0 {
		result.SensitivePolicyReviewRequired = true
		result.GoreeShieldReviewRequired = true
		result.WardveilReviewRequired = true
		result.ManualReviewRequired = true
		result.Blockers = append(result.Blockers, "image reference changes require Wardveil source/provenance and GoreeShield privacy review")
	}
	result.MIPSAuthorizationVerified = false
	return result, nil
}

// countChangedSecretReferenceServices counts affected service names without
// revealing any actual secret reference identifiers in the report.
func countChangedSecretReferenceServices(before, after Spec) int {
	current := make(map[string][]string, len(before.Services))
	target := make(map[string][]string, len(after.Services))
	for _, service := range before.Services {
		current[service.Name] = service.SecretRefs
	}
	for _, service := range after.Services {
		target[service.Name] = service.SecretRefs
	}
	seen := map[string]bool{}
	changed := 0
	for name, oldRefs := range current {
		seen[name] = true
		if !sameSecretReferences(oldRefs, target[name]) {
			changed++
		}
	}
	for name, newRefs := range target {
		if !seen[name] && len(newRefs) > 0 {
			changed++
		}
	}
	return changed
}
func sameSecretReferences(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]bool, len(left))
	for _, name := range left {
		seen[name] = true
	}
	for _, name := range right {
		if !seen[name] {
			return false
		}
	}
	return true
}

// Count changed service image references without returning any digest,
// URL or registry identity in user-visible error messages.
func countChangedImageReferenceServices(before, after Spec) int {
	current := map[string]string{}
	target := map[string]string{}
	for _, service := range before.Services {
		current[service.Name] = service.Image
	}
	for _, service := range after.Services {
		target[service.Name] = service.Image
	}
	changed := 0
	for name, digest := range current {
		if digest != target[name] {
			changed++
		}
	}
	for name := range target {
		if _, exists := current[name]; !exists {
			changed++
		}
	}
	return changed
}
