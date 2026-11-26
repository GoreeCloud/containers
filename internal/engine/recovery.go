package engine

import (
	"context"
	"errors"
	"os"
	"syscall"
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
		root, err := journalRoot(e.StateRoot)
		if err != nil {
			return err
		}
		defer root.Close()
		name := "journal/" + id + ".pending"
		before, err := root.Lstat(name)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		pending, err := ReadPendingMutation(ctx, e.StateRoot, id)
		if err != nil {
			return err
		}
		out.Pending = pending
		if !pending.Pending {
			if before != nil {
				return errors.New("pending marker disappeared before observation")
			}
			out.Observation = "no_pending_mutation"
			return nil
		}
		if before == nil {
			return errors.New("pending marker appeared before observation")
		}
		if err := verifyRecoveryMarker(root, name, before); err != nil {
			return err
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
		if err := verifyRecoveryMarker(root, name, before); err != nil {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		return RecoveryObservation{}, err
	}
	return out, nil
}

// Compare inode, metadata and Linux change time; a same-content replacement
// must not be treated as the previously observed recovery intent.
func verifyRecoveryMarker(root *os.Root, name string, before os.FileInfo) error {
	if err := verifyPendingIdentity(root, name, before); err != nil {
		return err
	}
	after, e := root.Lstat(name)
	if e != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("recovery marker changed")
	}
	a, ok := before.Sys().(*syscall.Stat_t)
	b, ok2 := after.Sys().(*syscall.Stat_t)
	if !ok || !ok2 || a.Ctim != b.Ctim {
		return errors.New("recovery marker change time differs")
	}
	return nil
}
