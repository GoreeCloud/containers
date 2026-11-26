package stack

import (
	"context"
	"errors"
	"time"
)

// LocalWatchSample is bound lifecycle evidence, not application health.
type LocalWatchSample struct {
	Sequence             int  `json:"sequence"`
	Expected             int  `json:"expected"`
	Running              int  `json:"running"`
	Stopped              int  `json:"stopped"`
	BindingsObserved     bool `json:"bindingsObserved"`
	AllRunningObserved   bool `json:"allRunningObserved"`
	ReviewRequired       bool `json:"reviewRequired"`
	ReadOnly             bool `json:"readOnly"`
	HealthVerified       bool `json:"healthVerified"`
	AutoRestart          bool `json:"autoRestart"`
	DeploymentAuthorized bool `json:"deploymentAuthorized"`
}

func WatchLocal(ctx context.Context, dir, pin, binary string, samples int, interval time.Duration, emit func(LocalWatchSample) error) error {
	return watchLocal(ctx, samples, interval, func(c context.Context) (LocalSessionReport, error) { return ManageLocal(c, dir, pin, binary, false) }, emit)
}

func watchLocal(ctx context.Context, samples int, interval time.Duration, observe func(context.Context) (LocalSessionReport, error), emit func(LocalWatchSample) error) error {
	if ctx == nil || observe == nil || emit == nil || samples < 1 || samples > 30 || interval < time.Second || interval > 10*time.Second {
		return errors.New("local watch requires 1–30 samples and 1–10 second interval")
	}
	for i := 1; i <= samples; i++ {
		if e := ctx.Err(); e != nil {
			return e
		}
		bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
		r, e := observe(bounded)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ok := e == nil && r.ReadOnly && r.BindingsObserved && !r.Pending && !r.RecoveryReviewRequired && r.Expected > 0 && r.Expected <= 4 && r.Running >= 0 && r.Stopped >= 0 && r.Running+r.Stopped == r.Expected
		sample := LocalWatchSample{Sequence: i, ReadOnly: true, ReviewRequired: true}
		if ok {
			sample.Expected = r.Expected
			sample.Running = r.Running
			sample.Stopped = r.Stopped
			sample.BindingsObserved = true
			sample.AllRunningObserved = r.Running == r.Expected
			sample.ReviewRequired = !sample.AllRunningObserved
		}
		if e := emit(sample); e != nil {
			return e
		}
		if sample.ReviewRequired {
			return errors.New("local workload liveness review required; no automatic restart")
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
