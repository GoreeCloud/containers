package stack

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLocalWatchFailsClosed(t *testing.T) {
	valid := LocalSessionReport{ReadOnly: true, Expected: 3, Running: 3, BindingsObserved: true}
	for _, c := range []struct {
		name string
		r    LocalSessionReport
		err  error
		pass bool
	}{
		{"valid", valid, nil, true}, {"stopped", LocalSessionReport{ReadOnly: true, Expected: 3, Running: 2, Stopped: 1, BindingsObserved: true}, nil, false}, {"unbound", LocalSessionReport{ReadOnly: true, Expected: 3, Running: 3}, nil, false}, {"inconsistent", LocalSessionReport{ReadOnly: true, Expected: 3, Running: 4, BindingsObserved: true}, nil, false}, {"error", valid, errors.New("private detail"), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			n := 0
			e := watchLocal(context.Background(), 1, time.Second, func(context.Context) (LocalSessionReport, error) { return c.r, c.err }, func(s LocalWatchSample) error {
				n++
				if s.AllRunningObserved != c.pass || s.ReviewRequired == c.pass || s.HealthVerified || s.AutoRestart || s.DeploymentAuthorized {
					t.Fatalf("unexpected sample %+v", s)
				}
				return nil
			})
			if (e == nil) != c.pass || n != 1 {
				t.Fatal(e, n)
			}
		})
	}
}
func TestLocalWatchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n := 0
	e := watchLocal(ctx, 2, time.Second, func(context.Context) (LocalSessionReport, error) {
		return LocalSessionReport{ReadOnly: true, Expected: 1, Running: 1, BindingsObserved: true}, nil
	}, func(s LocalWatchSample) error { n++; cancel(); return nil })
	if !errors.Is(e, context.Canceled) || n != 1 {
		t.Fatal(e, n)
	}
}
