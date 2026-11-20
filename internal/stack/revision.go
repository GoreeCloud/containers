package stack

import (
	"errors"
	"regexp"
)

// RevisionReview checks whether the caller-supplied expected revision matches
// the canonical fingerprint of a supplied "before" declaration. It does NOT
// inspect a live daemon, lock deployed state or authorize any mutation.
type RevisionReview struct {
	DryRun                    bool     `json:"dryRun"`
	ExpectedCurrentSHA256     string   `json:"expectedCurrentSHA256"`
	ObservedDeclarationSHA256 string   `json:"observedDeclarationSHA256"`
	ProposedSHA256            string   `json:"proposedSHA256"`
	MatchesExpected           bool     `json:"matchesExpected"`
	ChangeCount               int      `json:"changeCount"`
	ManualReviewRequired      bool     `json:"manualReviewRequired"`
	DeploymentAuthorized      bool     `json:"deploymentAuthorized"`
	LiveStateVerified         bool     `json:"liveStateVerified"`
	RollbackVerified          bool     `json:"rollbackVerified"`
	Blockers                  []string `json:"blockers"`
}

var revisionDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// ReviewRevision fails closed if the submitted expected hash does not match
// the source declaration, and continues to deny deployment even on a match.
// The future native controller must independently recheck an authenticated,
// durable running revision under its own exclusive rollout lock.
func ReviewRevision(before, after Spec, expected string) (RevisionReview, error) {
	var empty RevisionReview
	if !revisionDigestPattern.MatchString(expected) {
		return empty, errors.New("expected prior stack fingerprint must be a canonical SHA-256 descriptor")
	}
	changes, err := Diff(before, after)
	if err != nil {
		return empty, err
	}
	report := RevisionReview{
		DryRun: true, ExpectedCurrentSHA256: expected,
		ObservedDeclarationSHA256: changes.FromSpecSHA256,
		ProposedSHA256:            changes.ToSpecSHA256,
		MatchesExpected:           expected == changes.FromSpecSHA256,
		ChangeCount:               len(changes.Changes),
		ManualReviewRequired:      changes.ManualReviewRequired,
		DeploymentAuthorized:      false, LiveStateVerified: false, RollbackVerified: false,
		Blockers: make([]string, 0, 2),
	}
	if !report.MatchesExpected {
		report.ManualReviewRequired = true
		report.Blockers = append(report.Blockers, "caller-supplied expected revision differs from the prior declaration")
	}
	if report.ChangeCount > 0 {
		report.Blockers = append(report.Blockers, "native rollout requires a separate authorized executor and rollback approval")
	}
	return report, nil
}
