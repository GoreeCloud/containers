package engine

import (
	"context"
	"errors"
	"testing"
)

type recordingRuntimeRunner struct{ calls int }

func (r *recordingRuntimeRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	r.calls++
	return []byte("synthetic-ok"), nil
}
func TestRuntimeSafetyGateRecheckedBeforeEachOCICommand(t *testing.T) {
	fake := &recordingRuntimeRunner{}
	checks := 0
	fail := false
	r := guardedRuntimeRunner{
		expectedBinary: "/usr/bin/crun", delegate: fake,
		verify: func(_ context.Context, b string) error {
			checks++
			if b != "/usr/bin/crun" {
				t.Fatal("unexpected binary")
			}
			if fail {
				return errors.New("simulated binary downgrade")
			}
			return nil
		},
	}
	for i := 0; i < 2; i++ {
		if _, err := r.Run(context.Background(), "/usr/bin/crun", "state", "worker"); err != nil {
			t.Fatal(err)
		}
	}
	if checks != 2 || fake.calls != 2 {
		t.Fatalf("per-operation preflight skipped: %d %d", checks, fake.calls)
	}
	fail = true
	if _, err := r.Run(context.Background(), "/usr/bin/crun", "state", "worker"); err == nil {
		t.Fatal("mutated runtime version accepted")
	}
	if checks != 3 || fake.calls != 2 {
		t.Fatal("unsafe runtime command was executed after downgrade")
	}
	if _, err := r.Run(context.Background(), "/usr/bin/runc", "state", "worker"); err == nil {
		t.Fatal("changed runtime executable pathname accepted")
	}
	if checks != 3 || fake.calls != 2 {
		t.Fatal("different runtime path bypassed early block")
	}
}
func TestRuntimeSafetyGateRespectsCancelledContext(t *testing.T) {
	fake := &recordingRuntimeRunner{}
	r := guardedRuntimeRunner{
		expectedBinary: "/usr/bin/crun", delegate: fake,
		verify: func(context.Context, string) error { t.Fatal("validator invoked after cancellation"); return nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Run(ctx, "/usr/bin/crun", "state", "worker"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled OCI command not rejected: %v", err)
	}
	if fake.calls != 0 {
		t.Fatal("cancelled runtime command was delegated")
	}
}
