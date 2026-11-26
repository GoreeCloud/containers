package hostreadiness

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func testKernel() fstest.MapFS {
	return fstest.MapFS{
		"sys/fs/cgroup/cgroup.controllers":          &fstest.MapFile{Data: []byte("cpuset cpu io memory pids\n")},
		"proc/sys/kernel/unprivileged_userns_clone": &fstest.MapFile{Data: []byte("1\n")},
		"proc/sys/user/max_user_namespaces":         &fstest.MapFile{Data: []byte("128\n")},
	}
}
func TestReadOnlyKernelFactProbe(t *testing.T) {
	report, err := Inspect(context.Background(), testKernel(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReadOnly || !report.RootlessCaller || !report.CgroupV2Detected ||
		!report.ControllerCPU || !report.ControllerMemory || !report.ControllerPIDs ||
		!report.UserNamespaceSysctlEnabled || !report.UserNamespaceQuotaPositive {
		t.Fatalf("missing facts: %+v", report)
	}
	if report.IsolationVerified || report.DelegationVerified || report.DeploymentAuthorized {
		t.Fatal("kernel file presence must never certify isolation or deployment")
	}
}
func TestMissingRootlessPrerequisitesAreVisible(t *testing.T) {
	f := fstest.MapFS{
		"sys/fs/cgroup/cgroup.controllers":          &fstest.MapFile{Data: []byte("cpu\n")},
		"proc/sys/kernel/unprivileged_userns_clone": &fstest.MapFile{Data: []byte("0\n")},
		"proc/sys/user/max_user_namespaces":         &fstest.MapFile{Data: []byte("0\n")},
	}
	report, err := Inspect(context.Background(), f, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.RootlessCaller || report.UserNamespaceQuotaPositive || report.UserNamespaceSysctlEnabled ||
		report.ControllerMemory || report.ControllerPIDs || report.DeploymentAuthorized || len(report.Findings) < 4 {
		t.Fatalf("unready host admitted: %+v", report)
	}
}
func TestKernelFactSizeBoundAndCancellation(t *testing.T) {
	f := testKernel()
	f["sys/fs/cgroup/cgroup.controllers"] = &fstest.MapFile{Data: []byte(strings.Repeat("cpu ", 1500))}
	report, err := Inspect(context.Background(), f, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.CgroupV2Detected || report.DeploymentAuthorized {
		t.Fatal("oversized controller data was trusted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Inspect(ctx, testKernel(), true); err == nil {
		t.Fatal("canceled probe accepted")
	}
}
