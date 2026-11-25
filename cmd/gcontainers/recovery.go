package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func recoveryObserve(args []string, out io.Writer) error {
	if len(args) != 2 || (args[0] != "runc" && args[0] != "crun") {
		return errors.New("usage: recovery-observe runc|crun ID")
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return errors.New("private XDG_RUNTIME_DIR required")
	}
	real, err := filepath.EvalSymlinks(base)
	if err != nil || real != base {
		return errors.New("unsafe runtime directory")
	}
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	e, err := engine.New(binary, filepath.Join(base, "goreecloud-containers"), nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report, err := e.ObserveRecovery(ctx, args[1])
	if err != nil {
		return err
	}
	if err = json.NewEncoder(out).Encode(report); err != nil {
		return err
	}
	if report.Pending.RecoveryRequired {
		return engine.ErrUnresolvedMutation
	}
	return nil
}
