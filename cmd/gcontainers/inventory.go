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

func inventoryCommand(args []string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "runc" && args[0] != "crun") {
		return errors.New("usage: inventory runc|crun")
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	real, e := filepath.EvalSymlinks(base)
	if e != nil || !filepath.IsAbs(base) || filepath.Clean(base) != base || real != base {
		return errors.New("private runtime base required")
	}
	bin, e := exec.LookPath(args[0])
	if e != nil {
		return e
	}
	bin, e = filepath.Abs(bin)
	if e != nil {
		return e
	}
	runtime, e := engine.New(bin, filepath.Join(base, "goreecloud-containers"), nil)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, e := runtime.Inventory(ctx)
	if e != nil {
		return e
	}
	if e = json.NewEncoder(out).Encode(r); e != nil {
		return e
	}
	if r.RecoveryRequired {
		return engine.ErrUnresolvedMutation
	}
	return nil
}
