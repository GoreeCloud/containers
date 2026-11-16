// Package engine owns the GoreeCloud OCI lifecycle adapter. It never invokes a shell.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/GoreeCloud/containers/internal/bundle"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,62}$`)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type ProcessRunner struct{}

const maxRuntimeOutput = 64 << 10

type boundedOutput struct {
	data      []byte
	truncated bool
}

func (w *boundedOutput) Write(p []byte) (int, error) {
	original := len(p)
	remaining := maxRuntimeOutput - len(w.data)
	if remaining > 0 {
		count := remaining
		if count > len(p) {
			count = len(p)
		}
		w.data = append(w.data, p[:count]...)
	}
	if original > remaining {
		w.truncated = true
	}
	return original, nil
}
func filteredRuntimeEnv() []string {
	names := []string{"HOME", "LANG", "LC_ALL", "XDG_RUNTIME_DIR"}
	env := []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok && !strings.ContainsRune(value, 0) {
			env = append(env, name+"="+value)
		}
	}
	return env
}
func (ProcessRunner) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("process context required")
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = filteredRuntimeEnv()
	// Separate process group allows timeout to terminate descendant helper
	// processes that inherited the OCI runtime command's process group.
	// This does not provide complete OS process supervision/reaping.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	output := &boundedOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("OCI runtime operation failed: %w", err)
	}
	if output.truncated {
		return nil, errors.New("OCI runtime response exceeded 64 KiB")
	}
	return output.data, nil
}

type Engine struct {
	Binary        string
	StateRoot     string
	Runner        Runner
	lockMutations bool
}

func New(binary, stateRoot string, runner Runner) (*Engine, error) {
	if !filepath.IsAbs(binary) || !filepath.IsAbs(stateRoot) {
		return nil, errors.New("absolute runtime and state paths required")
	}
	name := filepath.Base(binary)
	if name != "runc" && name != "crun" {
		return nil, errors.New("only audited OCI runtime names crun/runc are permitted")
	}
	useLocks := runner == nil
	if runner == nil {
		// An actual process runner must satisfy the current fail-closed
		// external-runtime version and filesystem provenance baseline.
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := CheckInstalledRuntime(ctx, binary); err != nil {
			return nil, err
		}
		runner = ProcessRunner{}
	}
	return &Engine{Binary: binary, StateRoot: stateRoot, Runner: runner, lockMutations: useLocks}, nil
}
func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return errors.New("invalid container ID (lowercase 1-63 chars, starting with a letter)")
	}
	return nil
}
func (e *Engine) call(ctx context.Context, op string, args ...string) ([]byte, error) {
	argv := append([]string{"--root", e.StateRoot, op}, args...)
	if e.lockMutations && journalOp(op) {
		if len(args) == 0 {
			return nil, errors.New("OCI mutation container ID required")
		}
		id := args[0]
		if op == "create" {
			id = args[len(args)-1]
		}
		return RunJournaledMutation(ctx, e.StateRoot, id, op, func() ([]byte, error) {
			return e.Runner.Run(ctx, e.Binary, argv...)
		})
	}
	return e.Runner.Run(ctx, e.Binary, argv...)
}
func (e *Engine) Create(ctx context.Context, id, dir string) error {
	return e.mutate(ctx, id, func() error {

		if err := ValidateID(id); err != nil {
			return err
		}
		spec, err := bundle.Load(dir)
		if err != nil {
			return err
		}
		if err := bundle.RequireSingleCallerMapping(spec, uint32(os.Geteuid()), uint32(os.Getegid())); err != nil {
			return err
		}
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		_, err = e.call(ctx, "create", "--bundle", absolute, id)
		return err
	})
}
func (e *Engine) Start(ctx context.Context, id string) error {
	return e.mutate(ctx, id, func() error {

		state, err := e.readState(ctx, id)
		if err != nil {
			return err
		}
		if state.Status != "created" {
			return errors.New("OCI container must be in created state before start")
		}
		_, err = e.call(ctx, "start", id)
		return err
	})
}
func (e *Engine) State(ctx context.Context, id string) ([]byte, error) {
	var output []byte
	// Status reads use the same cooperative lock as state mutations.
	// An unresolved command must never be reported as a healthy state.
	err := e.mutate(ctx, id, func() error {
		if e.lockMutations {
			marker, err := ReadPendingMutation(ctx, e.StateRoot, id)
			if err != nil {
				return fmt.Errorf("OCI journal inspection failed: %w", err)
			}
			if marker.Pending || marker.RecoveryRequired {
				return ErrUnresolvedMutation
			}
		}
		state, err := e.readState(ctx, id)
		if err != nil {
			return err
		}
		output, err = json.Marshal(state)
		return err
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}
func (e *Engine) Stop(ctx context.Context, id string) error {
	return e.mutate(ctx, id, func() error {

		state, err := e.readState(ctx, id)
		if err != nil {
			return err
		}
		if state.Status != "running" {
			return errors.New("OCI container must be running before a stop request")
		}
		_, err = e.call(ctx, "kill", id, "SIGTERM")
		return err
	})
}
func (e *Engine) Delete(ctx context.Context, id string) error {
	return e.mutate(ctx, id, func() error {

		state, err := e.readState(ctx, id)
		if err != nil {
			return err
		}
		if state.Status != "created" && state.Status != "stopped" {
			return errors.New("running OCI container must not be deleted")
		}
		_, err = e.call(ctx, "delete", id) // Never force removal of running processes.
		return err
	})
}

// PrepareStateRoot restricts runtime metadata to the invoking unprivileged user.
// XDG_RUNTIME_DIR must be owned by the user and inaccessible to group/others.
func PrepareStateRoot(base string) (string, error) {
	if os.Geteuid() == 0 {
		return "", errors.New("root execution forbidden: use a rootless account")
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("XDG_RUNTIME_DIR must be absolute")
	}
	if err := privateDir(base); err != nil {
		return "", err
	}
	state := filepath.Join(base, "goreecloud-containers")
	if err := os.Mkdir(state, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err := privateDir(state); err != nil {
		return "", err
	}
	locks := filepath.Join(state, "locks")
	if err := os.Mkdir(locks, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err := privateDir(locks); err != nil {
		return "", err
	}
	journal := filepath.Join(state, "journal")
	if err := os.Mkdir(journal, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err := privateDir(journal); err != nil {
		return "", err
	}
	return state, nil
}
func privateDir(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode().Perm()&0077 != 0 {
		return errors.New("runtime state directory must be private, non-symlink directory")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return errors.New("runtime state directory must be user-owned")
	}
	return nil
}

// TrustedRuntimeBinary restricts the externally installed OCI runtime path
// to non-writable, root-owned system binaries. This does not audit its source.
func TrustedRuntimeBinary(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("OCI runtime must have a clean absolute path")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return errors.New("OCI runtime path could not be resolved")
	}
	if resolved != path && !strings.HasPrefix(path, "/bin/") {
		return errors.New("symlinked OCI runtime binary is unsupported")
	}
	dir := filepath.Dir(resolved)
	if dir != "/usr/bin" && dir != "/usr/local/bin" {
		return errors.New("OCI runtime must be installed in a trusted system directory")
	}
	if name := filepath.Base(resolved); name != "crun" && name != "runc" {
		return errors.New("unexpected low-level OCI executable")
	}
	fi, err := os.Lstat(resolved)
	if err != nil {
		return err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.Mode().IsRegular() || !ok || stat.Uid != 0 ||
		fi.Mode().Perm()&0022 != 0 || fi.Mode().Perm()&0111 == 0 {
		return errors.New("OCI runtime must be root-owned, executable and non-writable")
	}
	for _, check := range []string{dir, filepath.Dir(dir)} {
		info, err := os.Stat(check)
		if err != nil {
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || st.Uid != 0 || info.Mode().Perm()&0022 != 0 {
			return errors.New("OCI runtime directory permissions unsafe")
		}
	}
	return nil
}

// mutate holds the per-container cooperative lock for actual runtime
// mutations, including the read-state/validate/execute sequence. Mock runner
// tests bypass the production lock path; lock behavior has dedicated tests.
func (e *Engine) mutate(ctx context.Context, id string, operation func() error) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	if e.lockMutations {
		return WithContainerLock(ctx, e.StateRoot, id, operation)
	}
	return operation()
}
