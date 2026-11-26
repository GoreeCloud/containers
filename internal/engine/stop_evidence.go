package engine

import (
	"context"
	"errors"
	"time"
)

// A delivered SIGTERM is not a completed stop. Observe stopped PID zero
// before clearing the journal; never escalate automatically to SIGKILL.
func (e *Engine) waitStopped(ctx context.Context, id string, limit time.Duration) error {
	bounded, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	for {
		if err := bounded.Err(); err != nil {
			return errors.New("post-stop completion not verified")
		}
		state, err := e.readState(bounded, id)
		if err != nil {
			return errors.New("post-stop runtime state unavailable")
		}
		if state.Status == "stopped" && state.PID == 0 {
			return nil
		}
		if state.Status != "running" {
			return errors.New("unexpected post-stop runtime state")
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-bounded.Done():
			timer.Stop()
			return errors.New("post-stop completion not verified")
		case <-timer.C:
		}
	}
}
