// Package kernelprobe performs an inert first-party Linux rootless
// namespace creation check; it never executes an OCI runtime or image.
package kernelprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var names = []string{"user", "mnt", "pid", "ipc", "uts", "net"}

type snapshot struct {
	IDs map[string]string `json:"ids"`
	UID int               `json:"uid"`
}
type Report struct {
	ReadOnly                   bool     `json:"readOnly"`
	RootlessCaller             bool     `json:"rootlessCaller"`
	Attempted                  bool     `json:"attempted"`
	RootlessNamespacesDistinct bool     `json:"rootlessNamespacesDistinct"`
	UserMappedAsNamespaceRoot  bool     `json:"userMappedAsNamespaceRoot"`
	OCIIsolationVerified       bool     `json:"ociIsolationVerified"`
	SeccompVerified            bool     `json:"seccompVerified"`
	LSMVerified                bool     `json:"lsmVerified"`
	CgroupDelegationVerified   bool     `json:"cgroupDelegationVerified"`
	DeploymentAuthorized       bool     `json:"deploymentAuthorized"`
	Findings                   []string `json:"findings"`
}

func ReadSnapshot() (map[string]string, int, error) {
	ids := make(map[string]string, len(names))
	for _, name := range names {
		value, err := os.Readlink("/proc/self/ns/" + name)
		if err != nil {
			return nil, 0, err
		}
		ids[name] = value
	}
	return ids, os.Geteuid(), nil
}

// The isolated child receives parent namespace IDs over a bounded stdin pipe
// and returns only booleans. Inode identities are never emitted on stdout.
type childComparison struct {
	NamespacesDistinct bool `json:"namespacesDistinct"`
	UserMappedAsRoot   bool `json:"userMappedAsRoot"`
}

func WriteChildComparison(input io.Reader) ([]byte, error) {
	if input == nil {
		return nil, errors.New("parent snapshot required")
	}
	encoded, err := io.ReadAll(io.LimitReader(input, 2049))
	if err != nil || len(encoded) == 0 || len(encoded) > 2048 {
		return nil, errors.New("parent namespace snapshot invalid or too large")
	}
	var parent snapshot
	if err := json.Unmarshal(encoded, &parent); err != nil || parent.UID == 0 || len(parent.IDs) != len(names) {
		return nil, errors.New("invalid rootless parent namespace snapshot")
	}
	current, uid, err := ReadSnapshot()
	if err != nil {
		return nil, err
	}
	valid := Compare(parent.IDs, current, uid)
	return json.Marshal(childComparison{NamespacesDistinct: valid, UserMappedAsRoot: valid})
}

func Compare(parent, child map[string]string, childUID int) bool {
	if childUID != 0 || len(parent) != len(names) || len(child) != len(names) {
		return false
	}
	for _, kind := range names {
		if !validNamespaceRef(kind, parent[kind]) ||
			!validNamespaceRef(kind, child[kind]) || parent[kind] == child[kind] {
			return false
		}
	}
	return true
}

// validNamespaceRef requires the exact Linux /proc/self/ns/<kind> link form.
// Comparing two arbitrary unequal strings is not evidence of namespace
// isolation. The inode must be a positive, representable decimal uint64.
func validNamespaceRef(kind, ref string) bool {
	prefix := kind + ":["
	if !strings.HasPrefix(ref, prefix) || !strings.HasSuffix(ref, "]") {
		return false
	}
	inode := strings.TrimSuffix(strings.TrimPrefix(ref, prefix), "]")
	if inode == "" || len(inode) > 20 {
		return false
	}
	for _, ch := range inode {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	id, err := strconv.ParseUint(inode, 10, 64)
	return err == nil && id != 0
}

// Probe tests kernel namespace inode separation only. No OCI rootfs,
// proc mount, network interface, LSM, seccomp or cgroup policy is configured.
func Probe(ctx context.Context, executable string) (Report, error) {
	report := Report{ReadOnly: true, RootlessCaller: os.Geteuid() != 0,
		Findings: []string{"OCI rootfs confinement, seccomp, LSM, cgroups and runtime provenance unverified"}}
	if ctx == nil {
		return report, errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if os.Geteuid() == 0 || executable != "/proc/self/exe" {
		return report, errors.New("nonroot caller and absolute executable required")
	}
	original, _, err := ReadSnapshot()
	if err != nil {
		return report, err
	}
	timed, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	report.Attempted = true
	parentPayload, err := json.Marshal(snapshot{IDs: original, UID: os.Geteuid()})
	if err != nil || len(parentPayload) > 2048 {
		return report, errors.New("invalid parent namespace data")
	}
	cmd := exec.CommandContext(timed, executable, "__kernel-probe-child")
	cmd.Stdin = bytes.NewReader(parentPayload)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID |
			syscall.CLONE_NEWIPC | syscall.CLONE_NEWUTS | syscall.CLONE_NEWNET,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Geteuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getegid(), Size: 1}},
		GidMappingsEnableSetgroups: false, Pdeathsig: syscall.SIGKILL, Setpgid: true,
	}
	output := &boundedProbeOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	err = cmd.Run()
	if err != nil {
		report.Findings = append(report.Findings, "kernel namespace child denied or unavailable")
		return report, nil
	}
	if len(output.data) == 0 || output.truncated {
		report.Findings = append(report.Findings, "invalid bounded child response")
		return report, nil
	}
	var child childComparison
	if err := json.Unmarshal(output.data, &child); err != nil {
		report.Findings = append(report.Findings, "invalid isolated child response")
		return report, nil
	}
	report.RootlessNamespacesDistinct = child.NamespacesDistinct && child.UserMappedAsRoot
	report.UserMappedAsNamespaceRoot = report.RootlessNamespacesDistinct
	if !report.RootlessNamespacesDistinct {
		report.Findings = append(report.Findings, "not all requested namespaces were distinct")
	}
	return report, nil
}

// boundedProbeOutput never retains more than 4 KiB of a child process's
// stdout, including when a replaced executable unexpectedly emits more.
type boundedProbeOutput struct {
	data      []byte
	truncated bool
}

func (w *boundedProbeOutput) Write(p []byte) (int, error) {
	size := len(p)
	remaining := 4096 - len(w.data)
	if remaining > 0 {
		count := size
		if count > remaining {
			count = remaining
		}
		w.data = append(w.data, p[:count]...)
	}
	if size > remaining {
		w.truncated = true
	}
	return size, nil
}
