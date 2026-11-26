package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const safeSpec = `{
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

func setup(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "rootfs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestValidRootlessBundle(t *testing.T) {
	if err := Validate(setup(t, safeSpec)); err != nil {
		t.Fatal(err)
	}
}
func TestFailClosed(t *testing.T) {
	cases := map[string]string{
		"unknown field":     strings.Replace(safeSpec, `"ociVersion"`, `"surprise":true,"ociVersion"`, 1),
		"duplicate":         strings.Replace(safeSpec, `"ociVersion"`, `"ociVersion":"1.0.0","ociVersion"`, 1),
		"missing namespace": strings.Replace(safeSpec, `,{"type":"user"}`, `},`, 1),
		"host root mapping": strings.Replace(safeSpec, `"hostID":1000`, `"hostID":0`, 1),
		"no seccomp":        strings.Replace(safeSpec, `"defaultAction":"SCMP_ACT_ERRNO"`, `"defaultAction":"SCMP_ACT_ALLOW"`, 1),
		"writable root":     strings.Replace(safeSpec, `"readonly":true`, `"readonly":false`, 1),
		"capabilities":      strings.Replace(safeSpec, `"bounding":[]`, `"bounding":["CAP_SYS_ADMIN"]`, 1),
		"privilege":         strings.Replace(safeSpec, `"noNewPrivileges":true`, `"noNewPrivileges":false`, 1),
		"external root":     strings.Replace(safeSpec, `"path":"rootfs"`, `"path":"../rootfs"`, 1),
		"hook":              strings.Replace(safeSpec, `"root":`, `"hooks":{"prestart":[]},"root":`, 1),
		"bind mount":        strings.Replace(safeSpec, `"linux":`, `"mounts":[{"destination":"/tmp","type":"bind","source":"/"}],"linux":`, 1),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "missing namespace" {
				raw = strings.Replace(safeSpec, `{"type":"user"},`, "", 1)
			}
			if err := Validate(setup(t, raw)); err == nil {
				t.Fatal("insecure bundle accepted")
			}
		})
	}
}
func TestOCIWorkflowLayoutPreflightDoesNotAcceptMissingOrSymlinkedDevices(t *testing.T) {
	dir := setup(t, safeSpec)
	spec, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("accepted missing /dev and /proc as runtime-ready")
	}
	for _, name := range []string{"dev", "proc"} {
		if err := os.Mkdir(filepath.Join(dir, "rootfs", name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// A synthetically runnable entrypoint is required in addition to /dev
	// and /proc, without executing any guest process in this unit test.
	guestBase(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "rootfs", "bin", "true"), []byte("inert fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "rootfs", "dev")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp", filepath.Join(dir, "rootfs", "dev")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("accepted symlinked guest device directory")
	}
}

// Private device mount support is opt-in and intentionally narrower than
// the general runc spec. Never permit bind mounts or uncontrolled /dev access.
func TestPrivateDeviceMountAdmissionAndDenial(t *testing.T) {
	const privateDeviceMounts = `"mounts":[
      {"destination":"/dev","type":"tmpfs","source":"tmpfs",
       "options":["nosuid","noexec","nodev","mode=755","size=65536k"]},
      {"destination":"/dev/pts","type":"devpts","source":"devpts",
       "options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0620"]}
    ],`
	good := strings.Replace(safeSpec, `"linux":`, privateDeviceMounts+`"linux":`, 1)
	if err := Validate(setup(t, good)); err != nil {
		t.Fatalf("rejected exact optional private /dev profile: %v", err)
	}
	bad := map[string]string{
		"missing private /dev mount": strings.Replace(good,
			`{"destination":"/dev","type":"tmpfs","source":"tmpfs",
       "options":["nosuid","noexec","nodev","mode=755","size=65536k"]},`, "", 1),
		"missing paired devpts": strings.Replace(good,
			`"size=65536k"]},
      {"destination":"/dev/pts","type":"devpts","source":"devpts",
       "options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0620"]}`,
			`"size=65536k"]}`, 1),
		"unbounded /dev":               strings.Replace(good, `"size=65536k"`, `"size=9999999999"`, 1),
		"missing nodev":                strings.Replace(good, `"nodev",`, "", 1),
		"missing noexec":               strings.Replace(good, `"noexec",`, "", 1),
		"unsafe /dev source":           strings.Replace(good, `"source":"tmpfs"`, `"source":"/dev"`, 1),
		"bind host devices":            strings.Replace(good, `"type":"tmpfs"`, `"type":"bind"`, 1),
		"devpts without newinstance":   strings.Replace(good, `"newinstance",`, "", 1),
		"unreviewed devpts GID":        strings.Replace(good, `"mode=0620"`, `"gid=5"`, 1),
		"insecure ptmx mode":           strings.Replace(good, `"ptmxmode=0666"`, `"ptmxmode=0777"`, 1),
		"unapproved mount destination": strings.Replace(good, `"/dev/pts"`, `"/sys"`, 1),
		"duplicate /dev": strings.Replace(good, `"mounts":[`,
			`"mounts":[{"destination":"/dev","type":"tmpfs","source":"tmpfs",
       "options":["nosuid","noexec","nodev","mode=755","size=65536k"]},`, 1),
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			if raw == good {
				t.Fatal("negative fixture failed to mutate reviewed OCI profile")
			}
			if err := Validate(setup(t, raw)); err == nil {
				t.Fatal("unsafe private device profile was admitted")
			}
		})
	}
}

func TestPrivateDeviceMountRuntimeLayout(t *testing.T) {
	mounts := `"mounts":[
      {"destination":"/dev","type":"tmpfs","source":"tmpfs",
       "options":["nosuid","noexec","nodev","mode=755","size=65536k"]},
      {"destination":"/dev/pts","type":"devpts","source":"devpts",
       "options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0620"]}
    ],`
	dir := setup(t, strings.Replace(safeSpec, `"linux":`, mounts+`"linux":`, 1))
	spec, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"dev", "proc"} {
		if err := os.Mkdir(filepath.Join(dir, "rootfs", n), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("accepted missing /dev/pts with explicit devpts mount")
	}
	pts := filepath.Join(dir, "rootfs", "dev", "pts")
	if err := os.Mkdir(pts, 0700); err != nil {
		t.Fatal(err)
	}
	// A synthetically runnable entrypoint is required in addition to /dev
	// and /proc, without executing any guest process in this unit test.
	guestBase(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "rootfs", "bin", "true"), []byte("inert fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(pts); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/pts", pts); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("accepted symlinked guest /dev/pts mount destination")
	}
}

func TestTooLargeAndSymlink(t *testing.T) {
	dir := setup(t, safeSpec+strings.Repeat(" ", MaxConfigBytes))
	if err := Validate(dir); err == nil {
		t.Fatal("oversize accepted")
	}
	target := setup(t, safeSpec)
	link := filepath.Join(t.TempDir(), "config.json")
	if err := os.Symlink(filepath.Join(target, "config.json"), link); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(link, filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	if err := Validate(dir); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestMappingsRejectOverlaps(t *testing.T) {
	cases := [][]IDMap{
		{{ContainerID: 0, HostID: 1000, Size: 10}, {ContainerID: 10, HostID: 1005, Size: 10}},
		{{ContainerID: 0, HostID: 1000, Size: 10}, {ContainerID: 5, HostID: 2000, Size: 10}},
		{{ContainerID: 0, HostID: 0, Size: 1}},
		{{ContainerID: 0, HostID: 1000, Size: 0}},
	}
	for _, maps := range cases {
		if validMaps(maps) {
			t.Errorf("overlap/invalid mapping accepted: %+v", maps)
		}
	}
	if !validMaps([]IDMap{{ContainerID: 0, HostID: 1000, Size: 10}, {ContainerID: 10, HostID: 2000, Size: 10}}) {
		t.Fatal("non-overlapping mappings rejected")
	}
}
func TestPreviewMountAndSeccompRestrictions(t *testing.T) {
	cases := []string{
		strings.Replace(safeSpec, "\"linux\":", "\"mounts\":[{\"destination\":\"/tmp\",\"type\":\"tmpfs\",\"source\":\"tmpfs\",\"options\":[\"size=99999999\"]}],\"linux\":", 1),
		strings.Replace(safeSpec, "\"linux\":", "\"mounts\":[{\"destination\":\"/proc\",\"type\":\"proc\",\"source\":\"proc\",\"options\":[\"rw\"]}],\"linux\":", 1),
		strings.Replace(safeSpec, "\"defaultAction\":\"SCMP_ACT_ERRNO\"", "\"defaultAction\":\"SCMP_ACT_ERRNO\",\"syscalls\":[{\"names\":[\"mount\"],\"action\":\"SCMP_ACT_ALLOW\"}]", 1),
		strings.Replace(safeSpec, "\"defaultAction\":\"SCMP_ACT_ERRNO\"", "\"defaultAction\":\"SCMP_ACT_ERRNO\",\"flags\":[\"SECCOMP_FILTER_FLAG_TSYNC\"]", 1),
	}
	for _, raw := range cases {
		if err := Validate(setup(t, raw)); err == nil {
			t.Fatal("unsafe mount/seccomp config accepted")
		}
	}
}

func FuzzBundleStrictJSON(f *testing.F) {
	f.Add([]byte(safeSpec))
	f.Add([]byte("{\"a\":1,\"a\":2}"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8192 {
			t.Skip()
		}
		_ = rejectDuplicateKeys(b)
	})
}

func TestCallerOnlyMappingPolicy(t *testing.T) {
	spec, err := Load(setup(t, safeSpec))
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireSingleCallerMapping(spec, 1000, 1000); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][2]uint32{{0, 1000}, {1000, 0}, {2000, 1000}, {1000, 2000}} {
		if err := RequireSingleCallerMapping(spec, ids[0], ids[1]); err == nil {
			t.Fatalf("unapproved mapping accepted: %v", ids)
		}
	}
	spec.Linux.UIDMappings = append(spec.Linux.UIDMappings, IDMap{ContainerID: 1, HostID: 2000, Size: 1})
	if err := RequireSingleCallerMapping(spec, 1000, 1000); err == nil {
		t.Fatal("unsupported subordinate map accepted")
	}
}
func TestProcessPreviewRestrictions(t *testing.T) {
	cases := []*Process{
		{Args: []string{"relative"}, Cwd: "/"},
		{Args: []string{"/bin/../etc/passwd"}, Cwd: "/"},
		{Args: []string{"/bin/true"}, Cwd: "tmp"},
		{Args: []string{"/bin/true"}, Cwd: "/", Env: []string{"A=1", "A=2"}},
		{Args: []string{"/bin/true"}, Cwd: "/", Env: []string{"1BAD=x"}},
		{Args: []string{"/bin/true"}, Cwd: "/", Env: []string{"X"}},
		{Args: []string{"/bin/true"}, Cwd: "/", Env: []string{"A=good" + strings.Repeat("x", 4096)}},
	}
	for i, p := range cases {
		if err := validateProcess(p); err == nil {
			t.Fatalf("invalid process %d accepted", i)
		}
	}
	if err := validateProcess(&Process{Args: []string{"/bin/true"}, Cwd: "/", Env: []string{"LANG=C"}}); err != nil {
		t.Fatal(err)
	}
}

func TestBundleLoadRejectsSymlinkedRootAndAncestor(t *testing.T) {
	original := setup(t, safeSpec)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(original, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(link); err == nil {
		t.Fatal("symlinked OCI bundle accepted")
	}
	rootfs := filepath.Join(original, "rootfs")
	if err := os.Rename(rootfs, rootfs+"-real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("rootfs-real", rootfs); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(original); err == nil {
		t.Fatal("symlinked rootfs accepted")
	}
}
func TestBundleLoadRejectsSymlinkedIntermediateRootfs(t *testing.T) {
	raw := strings.Replace(safeSpec, `"path":"rootfs"`, `"path":"base/rootfs"`, 1)
	dir := setup(t, raw)
	if err := os.Mkdir(filepath.Join(dir, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "rootfs"), filepath.Join(dir, "real", "rootfs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "base")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("symlinked rootfs parent accepted")
	}
}
