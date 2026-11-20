package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const preflightOCI = `{
	"ociVersion":"1.2.0",
	"process":{"args":["/bin/true"],"cwd":"/","user":{"uid":0,"gid":0},
	"capabilities":{"bounding":[],"effective":[],"inheritable":[],"permitted":[],"ambient":[]},
	"noNewPrivileges":true,"terminal":false},
	"root":{"path":"rootfs","readonly":true},
	"linux":{"namespaces":[{"type":"user"},{"type":"pid"},{"type":"mount"},{"type":"network"},{"type":"ipc"},{"type":"uts"}],
	"uidMappings":[{"containerID":0,"hostID":1000,"size":1}],
	"gidMappings":[{"containerID":0,"hostID":1000,"size":1}],
	"seccomp":{"defaultAction":"SCMP_ACT_ERRNO"}}
}`

func TestBundlePreflightIsPrivateNonMutatingAndFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "rootfs"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfg, []byte(preflightOCI), 0600); err != nil {
		t.Fatal(err)
	}
	check := func(expectedPolicy, expectedLayout bool, reason string, wantError bool) {
		t.Helper()
		var out bytes.Buffer
		err := run([]string{"preflight", dir}, &out)
		if (err != nil) != wantError {
			t.Fatalf("unexpected status: %v; %s", err, out.String())
		}
		var report struct {
			ReadOnly             bool   `json:"readOnly"`
			PolicyValid          bool   `json:"policyValid"`
			GuestLayoutValid     bool   `json:"guestLayoutValid"`
			SeccompExecveListed  bool   `json:"seccompExecveListed"`
			RuntimeInvoked       bool   `json:"runtimeInvoked"`
			IsolationVerified    bool   `json:"isolationVerified"`
			DeploymentAuthorized bool   `json:"deploymentAuthorized"`
			Reason               string `json:"reason"`
		}
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if (report.SeccompExecveListed != (reason == "runtime_and_isolation_unverified")) ||
			!report.ReadOnly || report.RuntimeInvoked || report.IsolationVerified ||
			report.DeploymentAuthorized || report.PolicyValid != expectedPolicy ||
			report.GuestLayoutValid != expectedLayout || report.Reason != reason {
			t.Fatalf("unsafe or inconsistent result: %+v", report)
		}
		if strings.Contains(out.String(), dir) || strings.Contains(out.String(), "/bin/true") ||
			(err != nil && strings.Contains(err.Error(), dir)) {
			t.Fatal("preflight leaked sensitive bundle path")
		}
	}
	check(true, false, "guest_layout_rejected", true)
	for _, part := range []string{"dev", "proc", "bin"} {
		if err := os.Mkdir(filepath.Join(dir, "rootfs", part), 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(dir, "rootfs", "bin", "true")
	if err := os.WriteFile(binary, []byte("synthetic inert process"), 0700); err != nil {
		t.Fatal(err)
	}
	// The default-deny example is structurally valid but cannot execute a
	// guest process without an explicit execve permission.
	check(true, true, "seccomp_execve_unlisted", true)
	withExecve := strings.Replace(preflightOCI, `"defaultAction":"SCMP_ACT_ERRNO"`, `"defaultAction":"SCMP_ACT_ERRNO","syscalls":[{"names":["execve"],"action":"SCMP_ACT_ALLOW"}]`, 1)
	if withExecve == preflightOCI {
		t.Fatal("failed to construct positive seccomp fixture")
	}
	if err := os.WriteFile(cfg, []byte(withExecve), 0600); err != nil {
		t.Fatal(err)
	}
	check(true, true, "runtime_and_isolation_unverified", false)
	if err := os.WriteFile(cfg, []byte("{invalid-private-test-token}"), 0600); err != nil {
		t.Fatal(err)
	}
	check(false, false, "bundle_policy_rejected", true)
	contents, err := os.ReadFile(cfg)
	if err != nil || !bytes.Equal(contents, []byte("{invalid-private-test-token}")) {
		t.Fatal("read-only preflight modified configuration")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatal("read-only preflight changed guest rootfs")
	}
}

func TestBundlePreflightRejectsInvalidInvocation(t *testing.T) {
	for _, args := range [][]string{{"preflight"}, {"preflight", "a", "b"}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid CLI invocation admitted: %v", args)
		}
	}
}
