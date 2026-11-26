package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GoreeCloud/containers/internal/bundle"
)

// Bounded, read-only kernel evidence; never modify any host cgroup. Fail
// closed if runc silently ignores limits or the process cannot be observed.
// This proves configuration readback, not saturation/escape resistance.
func verifyResourceLimits(ctx context.Context, state RuntimeState, limits *bundle.Resources) error {
	if ctx == nil || ctx.Err() != nil || state.PID <= 0 || limits == nil || limits.CPU == nil || limits.Memory == nil || limits.Pids == nil {
		return errors.New("invalid OCI resource observation")
	}
	proc, err := os.OpenRoot("/proc/" + strconv.FormatInt(state.PID, 10))
	if err != nil {
		return errors.New("OCI resource process unavailable")
	}
	defer proc.Close()
	membership, err := boundedKernelRead(proc, "cgroup")
	if err != nil {
		return errors.New("OCI resource membership unavailable")
	}
	relative, err := resourceCgroupPath(membership, state.ID)
	if err != nil {
		return err
	}
	cg, err := os.OpenRoot("/sys/fs/cgroup")
	if err != nil {
		return errors.New("cgroup v2 unavailable")
	}
	defer cg.Close()
	observed, err := cg.OpenRoot(relative)
	if err != nil {
		return errors.New("OCI private cgroup unavailable")
	}
	defer observed.Close()
	values := map[string]string{
		"cpu.max":    fmt.Sprintf("%d %d", limits.CPU.Quota, limits.CPU.Period),
		"memory.max": strconv.FormatInt(limits.Memory.Limit, 10),
		"pids.max":   strconv.FormatInt(limits.Pids.Limit, 10),
	}
	for name, want := range values {
		if err := ctx.Err(); err != nil {
			return err
		}
		got, err := boundedKernelRead(observed, name)
		if err != nil || strings.Join(strings.Fields(got), " ") != want {
			return errors.New("OCI resource limit kernel readback mismatch")
		}
	}
	members, err := boundedKernelRead(observed, "cgroup.procs")
	if err != nil {
		return errors.New("OCI resource process membership unreadable")
	}
	found := false
	for _, pid := range strings.Fields(members) {
		if pid == strconv.FormatInt(state.PID, 10) {
			found = true
		}
	}
	if !found {
		return errors.New("OCI process absent from private cgroup")
	}
	after, err := boundedKernelRead(proc, "cgroup")
	if err != nil || after != membership {
		return errors.New("OCI process cgroup changed during observation")
	}
	return ctx.Err()
}
func boundedKernelRead(root *os.Root, name string) (string, error) {
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(b) > 65536 {
		return "", errors.New("kernel evidence exceeded bound")
	}
	return string(b), nil
}
func resourceCgroupPath(raw, id string) (string, error) {
	if err := ValidateID(id); err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "0::/") {
		return "", errors.New("expected single unified cgroup v2 membership")
	}
	path := strings.TrimPrefix(lines[0], "0::/")
	if !filepath.IsLocal(path) || filepath.Clean(path) != path || filepath.Base(path) != id || path == id {
		return "", errors.New("OCI process not in expected private cgroup")
	}
	return path, nil
}
