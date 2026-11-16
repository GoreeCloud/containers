package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeRunner struct {
	argv []string
	fail bool
}

func (f *fakeRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.argv = append([]string(nil), args...)
	if f.fail {
		return nil, errors.New("runtime failed")
	}
	return []byte(`{"status":"created"}`), nil
}
func TestIdentifiers(t *testing.T) {
	for _, id := range []string{"", "../x", "Upper", "a/b", "-oops", "a b"} {
		if ValidateID(id) == nil {
			t.Errorf("accepted %q", id)
		}
	}
	if err := ValidateID("server_01"); err != nil {
		t.Fatal(err)
	}
}
func TestStateAndLifecycle(t *testing.T) {
	f := &fakeRunner{}
	e, err := New("/usr/bin/crun", "/tmp/gc-test", f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.State(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if len(f.argv) != 4 || f.argv[2] != "state" || f.argv[3] != "a" {
		t.Fatalf("incorrect invocation: %v", f.argv)
	}
	if err := e.Stop(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if f.argv[2] != "kill" || f.argv[4] != "SIGTERM" {
		t.Fatalf("unsafe stop: %v", f.argv)
	}
	if err := e.Delete(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if f.argv[2] != "delete" || len(f.argv) != 4 {
		t.Fatalf("force delete not allowed: %v", f.argv)
	}
	f.fail = true
	if err := e.Start(context.Background(), "a"); err == nil {
		t.Fatal("lost runtime error")
	}
	if err := e.Start(context.Background(), "../escape"); err == nil {
		t.Fatal("bad ID passed")
	}
}
func TestStateRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless test requires nonroot")
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := PrepareStateRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	if state != filepath.Join(base, "goreecloud-containers") {
		t.Fatal(state)
	}
	if err := os.Chmod(base, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareStateRoot(base); err == nil {
		t.Fatal("public runtime dir accepted")
	}
}
