package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

type RecoveryAssessment struct {
	ID                    string `json:"id"`
	PendingOperation      string `json:"pendingOperation,omitempty"`
	ObservedState         string `json:"observedState"`
	ReviewCategory        string `json:"reviewCategory"`
	ObservationSHA256     string `json:"observationSHA256"`
	PostconditionObserved bool   `json:"postconditionObserved"`
	RecoveryRequired      bool   `json:"recoveryRequired"`
	ReadOnly              bool   `json:"readOnly"`
	IdentityVerified      bool   `json:"identityVerified"`
	RecoveryAuthorized    bool   `json:"recoveryAuthorized"`
	JournalCleared        bool   `json:"journalCleared"`
}

func assessRecovery(o RecoveryObservation) (RecoveryAssessment, error) {
	r := RecoveryAssessment{ID: o.Pending.ID, ReadOnly: true, ObservedState: "unavailable", ReviewCategory: "no_pending_operation"}
	if ValidateID(r.ID) != nil || !o.Pending.ReadOnly || o.RecoveryAuthorized {
		return RecoveryAssessment{}, errors.New("invalid recovery observation")
	}
	if !o.Pending.Pending {
		if o.Pending.Operation != "" || o.Observed != nil || o.Pending.RecoveryRequired {
			return RecoveryAssessment{}, errors.New("inconsistent empty observation")
		}
	} else {
		if !journalOp(o.Pending.Operation) || !o.Pending.RecoveryRequired {
			return RecoveryAssessment{}, errors.New("invalid pending operation")
		}
		r.PendingOperation = o.Pending.Operation
		r.RecoveryRequired = true
		r.ReviewCategory = "runtime_unavailable_preserve_evidence"
		if o.Observed != nil {
			s := o.Observed
			if s.ID != r.ID || s.PID < 0 || (s.Status != "created" && s.Status != "running" && s.Status != "stopped") {
				return RecoveryAssessment{}, errors.New("invalid observed lifecycle")
			}
			r.ObservedState = s.Status
			r.ReviewCategory = "state_conflict_or_incomplete_review"
			switch r.PendingOperation {
			case "create":
				r.PostconditionObserved = s.Status == "created" && s.PID > 0
			case "start":
				r.PostconditionObserved = s.Status == "running" && s.PID > 0
			case "kill":
				r.PostconditionObserved = s.Status == "stopped" && s.PID == 0
			case "delete":
				r.ReviewCategory = "metadata_or_runtime_still_present_review"
			}
			if r.PostconditionObserved {
				r.ReviewCategory = "postcondition_observed_identity_unverified"
			}
		}
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return RecoveryAssessment{}, e
	}
	r.ObservationSHA256 = fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	return r, nil
}

// AssessRecovery never authorizes repair. The digest pins the redacted
// assessment, not the runtime process identity, journal inode or future state.
func (e *Engine) AssessRecovery(ctx context.Context, id, expected string) (RecoveryAssessment, error) {
	observation, err := e.ObserveRecovery(ctx, id)
	if err != nil {
		return RecoveryAssessment{}, err
	}
	r, err := assessRecovery(observation)
	if err != nil {
		return RecoveryAssessment{}, err
	}
	if expected != "" && expected != r.ObservationSHA256 {
		return RecoveryAssessment{}, errors.New("recovery assessment changed")
	}
	return r, nil
}
