package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	argv   []string
	fail   bool
	status string
}

func (f *fakeRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.argv = append([]string(nil), args...)
	if f.fail {
		return nil, errors.New("runtime failed")
	}
	if len(args) < 4 {
		return nil, errors.New("missing operation or id")
	}
	if f.status == "" {
		f.status = "created"
	}
	switch args[2] {
	case "state":
		pid := 0
		if f.status == "running" {
			pid = 4321
		}
		return []byte(fmt.Sprintf(`{"id":%q,"status":%q,"pid":%d}`, args[3], f.status, pid)), nil
	case "start":
		f.status = "running"
	case "kill":
		f.status = "stopped"
	case "delete":
		f.status = "stopped"
	}
	return []byte("ok"), nil
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
	f.status = "running"
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

func TestBoundedRuntimeOutput(t *testing.T) {
	output := &boundedOutput{}
	payload := make([]byte, maxRuntimeOutput+4096)
	n, err := output.Write(payload)
	if err != nil || n != len(payload) || !output.truncated || len(output.data) != maxRuntimeOutput {
		t.Fatal("bounded output did not truncate safely")
	}
}
func TestRuntimeEnvironmentIsScoped(t *testing.T) {
	t.Setenv("GOREECLOUD_TEST_CREDENTIAL", "secret-canary")
	for _, value := range filteredRuntimeEnv() {
		if strings.Contains(value, "secret-canary") || strings.HasPrefix(value, "GOREECLOUD_TEST_CREDENTIAL=") {
			t.Fatal("runtime environment leaked unrelated variable")
		}
	}
}

func TestRejectUntrustedExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crun")
	if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := TrustedRuntimeBinary(path); err == nil {
		t.Fatal("untrusted user executable accepted")
	}
	if err := TrustedRuntimeBinary("../crun"); err == nil {
		t.Fatal("relative runtime path accepted")
	}
}

func TestRuntimeChildEnvironmentDoesNotInheritPath(t *testing.T) {
	t.Setenv("PATH", "/tmp/untrusted-search-path")
	found := false
	for _, entry := range filteredRuntimeEnv() {
		if entry == "PATH=/usr/sbin:/usr/bin:/sbin:/bin" {
			found = true
		}
		if strings.Contains(entry, "untrusted-search-path") {
			t.Fatal("runtime inherited attacker-controlled PATH")
		}
	}
	if !found {
		t.Fatal("trusted PATH missing")
	}
}
func TestProcessRunnerHonorsCancellation(t *testing.T) {
	path := "/usr/bin/sleep"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no trusted system sleep binary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := (ProcessRunner{}).Run(ctx, path, "10"); err == nil {
		t.Fatal("timeout unexpectedly succeeded")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("OCI subprocess cancellation exceeded bound")
	}
}
func TestProcessRunnerRejectsNilContext(t *testing.T) {
	if _, err := (ProcessRunner{}).Run(nil, "/usr/bin/true"); err == nil {
		t.Fatal("nil context accepted")
	}
}

func TestLifecycleRejectsInvalidTransitions(t *testing.T) {
	fake := &fakeRunner{status: "running"}
	eng, err := New("/usr/bin/crun", "/tmp/synthetic-private-state", fake)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Start(context.Background(), "worker"); err == nil {
		t.Fatal("start accepted for running container")
	}
	if err := eng.Delete(context.Background(), "worker"); err == nil {
		t.Fatal("running container delete accepted")
	}
	fake.status = "stopped"
	if err := eng.Stop(context.Background(), "worker"); err == nil {
		t.Fatal("stop accepted for stopped container")
	}
	if err := eng.Start(context.Background(), "worker"); err == nil {
		t.Fatal("start accepted for stopped container")
	}
	fake.status = "created"
	if err := eng.Start(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
	if fake.status != "running" {
		t.Fatal("start not dispatched")
	}
	if err := eng.Stop(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
	if fake.status != "stopped" {
		t.Fatal("stop not dispatched")
	}
	if err := eng.Delete(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
}
