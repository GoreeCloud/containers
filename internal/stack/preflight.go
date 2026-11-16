package stack

import (
	"errors"
)

// PreflightReport composes independent source-document guards over the SAME
// parsed old/new declarations. It does not inspect actual deployed workloads
// and never authorizes their creation, update or removal.
type PreflightReport struct {
	DryRun                      bool                     `json:"dryRun"`
	SourceAndBudgetChecksPassed bool                     `json:"sourceAndBudgetChecksPassed"`
	DeploymentAuthorized        bool                     `json:"deploymentAuthorized"`
	LiveStateVerified           bool                     `json:"liveStateVerified"`
	RollbackVerified            bool                     `json:"rollbackVerified"`
	ManualReviewRequired        bool                     `json:"manualReviewRequired"`
	Revision                    RevisionReview           `json:"revision"`
	Capacity                    TransitionCapacityReview `json:"capacity"`
	Impact                      ImpactReport             `json:"impact"`
	Blockers                    []string                 `json:"blockers"`
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
	return result, nil
}
