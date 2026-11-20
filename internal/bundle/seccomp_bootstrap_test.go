package bundle

import (
	"encoding/json"
	"testing"
)

func TestGuestExecveBootstrapPreflightFailsClosed(t *testing.T) {
	makeSpec := func(syscalls string) *Spec {
		t.Helper()
		var s Spec
		raw := `{"linux":{"seccomp":{"defaultAction":"SCMP_ACT_ERRNO","syscalls":` + syscalls + `}}}`
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			t.Fatal(err)
		}
		return &s
	}
	for _, bad := range []string{
		`[]`,
		`[{"names":["openat","exit"],"action":"SCMP_ACT_ALLOW"}]`,
		`[{"names":["execve"],"action":"SCMP_ACT_ERRNO"}]`,
		`[{"names":["execve"],"action":"SCMP_ACT_KILL_PROCESS"}]`,
	} {
		if HasExplicitGuestExecveAllowance(makeSpec(bad)) {
			t.Fatalf("unlisted or denied guest execve looked permitted: %s", bad)
		}
	}
	if !HasExplicitGuestExecveAllowance(makeSpec(`[{"names":["execve"],"action":"SCMP_ACT_ALLOW"}]`)) {
		t.Fatal("explicitly allowed execve not recognized")
	}
	if HasExplicitGuestExecveAllowance(nil) || HasExplicitGuestExecveAllowance(&Spec{}) {
		t.Fatal("missing seccomp policy admitted")
	}
}
