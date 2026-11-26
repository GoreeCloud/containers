package hostreadiness

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strconv"
	"strings"
)

// Report is a non-identifying, read-only set of kernel facts; it never
// authorizes workloads or claims that namespace isolation was tested.
type Report struct {
	ReadOnly                   bool     `json:"readOnly"`
	RootlessCaller             bool     `json:"rootlessCaller"`
	CgroupV2Detected           bool     `json:"cgroupV2Detected"`
	ControllerCPU              bool     `json:"controllerCpu"`
	ControllerMemory           bool     `json:"controllerMemory"`
	ControllerPIDs             bool     `json:"controllerPids"`
	UserNamespaceSysctlEnabled bool     `json:"userNamespaceSysctlEnabled"`
	UserNamespaceQuotaPositive bool     `json:"userNamespaceQuotaPositive"`
	DelegationVerified         bool     `json:"delegationVerified"`
	IsolationVerified          bool     `json:"isolationVerified"`
	DeploymentAuthorized       bool     `json:"deploymentAuthorized"`
	Findings                   []string `json:"findings"`
}

// Inspect reads only fixed proc/sysfs paths through an injected fs.FS.
// Availability of controllers does NOT establish user delegation.
func Inspect(ctx context.Context, filesystem fs.FS, nonroot bool) (Report, error) {
	if ctx == nil || filesystem == nil {
		return Report{}, errors.New("context and filesystem required")
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	result := Report{ReadOnly: true, RootlessCaller: nonroot, Findings: make([]string, 0, 7)}
	controllers, err := readKernelFact(ctx, filesystem, "sys/fs/cgroup/cgroup.controllers")
	if err == nil {
		result.CgroupV2Detected = true
		for _, name := range strings.Fields(controllers) {
			switch name {
			case "cpu":
				result.ControllerCPU = true
			case "memory":
				result.ControllerMemory = true
			case "pids":
				result.ControllerPIDs = true
			}
		}
	} else {
		result.Findings = append(result.Findings, "cgroup v2 controllers unavailable or invalid")
	}
	switchFlag, err := readKernelFact(ctx, filesystem, "proc/sys/kernel/unprivileged_userns_clone")
	result.UserNamespaceSysctlEnabled = err == nil && strings.TrimSpace(switchFlag) == "1"
	if !result.UserNamespaceSysctlEnabled {
		result.Findings = append(result.Findings, "unprivileged user namespace sysctl not confirmed enabled")
	}
	quota, err := readKernelFact(ctx, filesystem, "proc/sys/user/max_user_namespaces")
	if err == nil {
		parsed, parseErr := strconv.ParseUint(strings.TrimSpace(quota), 10, 64)
		result.UserNamespaceQuotaPositive = parseErr == nil && parsed > 0
	}
	if !result.UserNamespaceQuotaPositive {
		result.Findings = append(result.Findings, "namespace quota not confirmed positive")
	}
	if !nonroot {
		result.Findings = append(result.Findings, "caller is root, not eligible for rootless admission")
	}
	if !(result.ControllerCPU && result.ControllerMemory && result.ControllerPIDs) {
		result.Findings = append(result.Findings, "required cpu/memory/pids controllers not all observed")
	}
	result.Findings = append(result.Findings, "cgroup delegation, runtime provenance, sandbox isolation and rollback remain unverified")
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	return result, nil
}
func readKernelFact(ctx context.Context, filesystem fs.FS, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := filesystem.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(b) > 4096 {
		return "", errors.New("kernel fact exceeds read quota")
	}
	return string(b), nil
}
