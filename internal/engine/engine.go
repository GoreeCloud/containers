// Package engine owns the GoreeCloud OCI lifecycle adapter. It never invokes a shell.
package engine

import (
	"context"
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
	names := []string{"HOME", "PATH", "LANG", "LC_ALL", "XDG_RUNTIME_DIR"}
	env := make([]string, 0, len(names))
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}
func (ProcessRunner) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = filteredRuntimeEnv()
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
	Binary    string
	StateRoot string
	Runner    Runner
}

func New(binary, stateRoot string, runner Runner) (*Engine, error) {
	if !filepath.IsAbs(binary) || !filepath.IsAbs(stateRoot) {
		return nil, errors.New("absolute runtime and state paths required")
	}
	name := filepath.Base(binary)
	if name != "runc" && name != "crun" {
		return nil, errors.New("only audited OCI runtime names crun/runc are permitted")
	}
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
	return &Engine{Binary: binary, StateRoot: stateRoot, Runner: runner}, nil
}
func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return errors.New("invalid container ID (lowercase 1-63 chars, starting with a letter)")
	}
	return nil
}
func (e *Engine) call(ctx context.Context, op string, args ...string) ([]byte, error) {
	argv := append([]string{"--root", e.StateRoot, op}, args...)
	return e.Runner.Run(ctx, e.Binary, argv...)
}
func (e *Engine) Create(ctx context.Context, id, dir string) error {
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
}
func (e *Engine) Start(ctx context.Context, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	_, err := e.call(ctx, "start", id)
	return err
}
func (e *Engine) State(ctx context.Context, id string) ([]byte, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	return e.call(ctx, "state", id)
}
func (e *Engine) Stop(ctx context.Context, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	_, err := e.call(ctx, "kill", id, "SIGTERM")
	return err
}
func (e *Engine) Delete(ctx context.Context, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	_, err := e.call(ctx, "delete", id) // Never force removal of a running container.
	return err
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
