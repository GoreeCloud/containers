package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Exercises the real-runner safety path with an injected fake OCI runtime:
// no external process or container is launched.
func TestStateReadRefusesUnresolvedMutation(t *testing.T) {
	root := journalFixture(t)
	f := &fakeRunner{}
	e, err := New("/usr/bin/crun", root, f)
	if err != nil {
		t.Fatal(err)
	}
	e.lockMutations = true
	clean, err := e.State(context.Background(), "worker")
	if err != nil || len(clean) == 0 {
		t.Fatalf("baseline state unavailable: %v", err)
	}
	_, err = RunJournaledMutation(context.Background(), root, "worker", "start", func() ([]byte, error) {
		return nil, errors.New("synthetic failed start")
	})
	if !errors.Is(err, ErrUnresolvedMutation) {
		t.Fatal(err)
	}
	prior := append([]string(nil), f.argv...)
	state, err := e.State(context.Background(), "worker")
	if !errors.Is(err, ErrUnresolvedMutation) || state != nil {
		t.Fatalf("unresolved state was exposed: %s %v", state, err)
	}
	if len(f.argv) != len(prior) {
		t.Fatal("external state operation ran despite unresolved journal")
	}
	marker, err := ReadPendingMutation(context.Background(), root, "worker")
	if err != nil || !marker.RecoveryRequired {
		t.Fatal("state read cleared an unresolved mutation")
	}
}
func TestStateReadWaitsForCooperativeMutationLock(t *testing.T) {
	root := journalFixture(t)
	e, err := New("/usr/bin/crun", root, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	e.lockMutations = true
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithContainerLock(context.Background(), root, "worker", func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()
	if _, err := e.State(ctx, "worker"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("state read bypassed competing mutation lock: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := e.State(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
}
