package engine

import (
	"context"
	"errors"
	"time"
)

type WatchSample struct {
	Sequence         int  `json:"sequence"`
	Total            int  `json:"total"`
	Running          int  `json:"running"`
	Created          int  `json:"created"`
	Stopped          int  `json:"stopped"`
	Uncertain        int  `json:"uncertain"`
	RecoveryRequired bool `json:"recoveryRequired"`
	ReadOnly         bool `json:"readOnly"`
	AtomicSnapshot   bool `json:"atomicSnapshot"`
	AutoRepair       bool `json:"autoRepair"`
}

// Watch emits bounded, aggregate observations only. It never retries an
// uncertain census as healthy, writes a durable log or restarts a workload.
func (e *Engine) Watch(ctx context.Context, samples int, interval time.Duration, emit func(WatchSample) error) error {
	if ctx == nil || emit == nil || samples < 1 || samples > 60 || interval < time.Second || interval > 10*time.Second {
		return errors.New("watch requires 1–60 samples and 1–10 second interval")
	}
	for i := 1; i <= samples; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		r, err := e.Inventory(bounded)
		cancel()
		if err != nil {
			return errors.New("runtime watch census unavailable; stop and review")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		s := WatchSample{Sequence: i, Total: len(r.Entries), Running: r.Running, Created: r.Created, Stopped: r.Stopped, Uncertain: r.Uncertain, RecoveryRequired: r.RecoveryRequired, ReadOnly: true}
		if err = emit(s); err != nil {
			return err
		}
		if s.RecoveryRequired {
			return ErrUnresolvedMutation
		}
		if i < samples {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}
