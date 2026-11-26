package engine

import (
	"context"
	"errors"
	"time"
)

// WaitStopped observes lifecycle state without signaling a guest or clearing
// journals. It releases the per-ID cooperative lock between reads.
type WaitReport struct {
	ID                       string `json:"id"`
	Stopped                  bool   `json:"stopped"`
	ObservedRunning          bool   `json:"observedRunning"`
	ExitCodeVerified         bool   `json:"exitCodeVerified"`
	InstanceIdentityVerified bool   `json:"instanceIdentityVerified"`
	ReadOnly                 bool   `json:"readOnly"`
}

func (e *Engine) WaitStopped(ctx context.Context, id string, limit time.Duration) (WaitReport, error) {
	var empty WaitReport
	if ctx == nil || limit < time.Second || limit > 60*time.Second {
		return empty, errors.New("wait requires a 1–60 second bound")
	}
	if err := ValidateID(id); err != nil {
		return empty, err
	}
	bounded, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	report := WaitReport{ID: id, ReadOnly: true}
	var pid int64
	for {
		if err := bounded.Err(); err != nil {
			return empty, err
		}
		raw, err := e.State(bounded, id)
		if err != nil {
			return empty, err
		}
		state, err := DecodeRuntimeState(raw, id)
		if err != nil {
			return empty, err
		}
		if err = bounded.Err(); err != nil {
			return empty, err
		}
		switch state.Status {
		case "stopped":
			if state.PID != 0 {
				return empty, errors.New("stopped PID not zero")
			}
			report.Stopped = true
			return report, nil
		case "running":
			if pid != 0 && state.PID != pid {
				return empty, errors.New("observed runtime PID changed")
			}
			pid = state.PID
			report.ObservedRunning = true
		default:
			return empty, errors.New("wait accepts only running or stopped state")
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-bounded.Done():
			timer.Stop()
			return empty, bounded.Err()
		case <-timer.C:
		}
	}
}
