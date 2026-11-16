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
	"syscall"

	"github.com/GoreeCloud/containers/internal/bundle"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,62}$`)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type ProcessRunner struct{}

func (ProcessRunner) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("OCI runtime operation failed: %w", err)
	}
	return out, nil
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
	if err := bundle.Validate(dir); err != nil {
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
