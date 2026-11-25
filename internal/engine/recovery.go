package engine

import (
	"context"
	"errors"
)

// RecoveryObservation correlates a pending operation with a bounded runtime
// state read. It never clears the marker, signals a process, or treats a
// missing/failed runtime response as proof of deletion.
type RecoveryObservation struct {
	Pending            PendingMutation `json:"pending"`
	Observed           *RuntimeState   `json:"observed,omitempty"`
	Observation        string          `json:"observation"`
	RecoveryAuthorized bool            `json:"recoveryAuthorized"`
}

func (e *Engine) ObserveRecovery(ctx context.Context, id string) (out RecoveryObservation, err error) {
	if ctx == nil {
		return out, errors.New("context required")
	}
	err = WithContainerLock(ctx, e.StateRoot, id, func() error {
		pending, err := ReadPendingMutation(ctx, e.StateRoot, id)
		if err != nil {
			return err
		}
		out.Pending = pending
		if !pending.Pending {
			out.Observation = "no_pending_mutation"
			return nil
		}
		state, err := e.readState(ctx, id)
		if err != nil {
			out.Observation = "runtime_state_unavailable"
		} else {
			out.Observed = &state
			out.Observation = "runtime_state_observed_not_reconciled"
		}
		again, err := ReadPendingMutation(ctx, e.StateRoot, id)
		if err != nil || again != pending {
			return errors.New("pending mutation changed during recovery observation")
		}
		return ctx.Err()
	})
	if err != nil {
		return RecoveryObservation{}, err
	}
	return out, nil
}
