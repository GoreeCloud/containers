package engine

import (
	"context"
	"errors"
	"reflect"
)

type RecoverySummary struct {
	ReadOnly              bool `json:"readOnly"`
	PendingCount          int  `json:"pendingCount"`
	PostconditionObserved int  `json:"postconditionObserved"`
	StateConflict         int  `json:"stateConflict"`
	RuntimeUnavailable    int  `json:"runtimeUnavailable"`
	DeleteReview          int  `json:"deleteReview"`
	RecoveryRequired      bool `json:"recoveryRequired"`
	AtomicSnapshot        bool `json:"atomicSnapshot"`
	RecoveryAuthorized    bool `json:"recoveryAuthorized"`
}

// SummarizeRecovery uses two inventory observations and a bounded set of
// anchored per-ID recovery reads. Never expose IDs, paths, PIDs or digests.
func (e *Engine) SummarizeRecovery(ctx context.Context) (RecoverySummary, error) {
	var empty RecoverySummary
	if ctx == nil {
		return empty, errors.New("context required")
	}
	first, err := e.Inventory(ctx)
	if err != nil {
		return empty, err
	}
	r := RecoverySummary{ReadOnly: true, RecoveryRequired: first.RecoveryRequired}
	for _, x := range first.Entries {
		if x.PendingOperation != "" {
			r.PendingCount++
		}
	}
	if r.PendingCount > 16 {
		return empty, errors.New("recovery assessment bound exceeded")
	}
	for _, x := range first.Entries {
		if x.PendingOperation == "" {
			continue
		}
		a, err := e.AssessRecovery(ctx, x.ID, "")
		if err != nil || !a.RecoveryRequired || a.PendingOperation != x.PendingOperation {
			return empty, errors.New("recovery observation changed")
		}
		if a.ObservedState != "unavailable" && a.ObservedState != x.Status {
			return empty, errors.New("recovery state changed")
		}
		switch a.ReviewCategory {
		case "postcondition_observed_identity_unverified":
			r.PostconditionObserved++
		case "runtime_unavailable_preserve_evidence":
			if x.Status != "uncertain" {
				return empty, errors.New("runtime availability changed")
			}
			r.RuntimeUnavailable++
		case "metadata_or_runtime_still_present_review":
			r.DeleteReview++
		case "state_conflict_or_incomplete_review":
			r.StateConflict++
		default:
			return empty, errors.New("unexpected recovery assessment category")
		}
	}
	second, err := e.Inventory(ctx)
	if err != nil || !reflect.DeepEqual(first, second) {
		return empty, errors.New("runtime inventory changed during assessment")
	}
	if r.PendingCount != r.PostconditionObserved+r.StateConflict+r.RuntimeUnavailable+r.DeleteReview {
		return empty, errors.New("inconsistent recovery summary")
	}
	return r, ctx.Err()
}
