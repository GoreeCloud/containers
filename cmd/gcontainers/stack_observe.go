package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"github.com/GoreeCloud/containers/internal/stack"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func stackObserve(args []string, out io.Writer) error {
	if len(args) != 4 || args[2] != "--expected-plan-sha256" || (args[1] != "runc" && args[1] != "crun") {
		return errors.New("usage: stack observe SPEC runc|crun --expected-plan-sha256 DIGEST")
	}
	spec, e := stack.Load(args[0])
	if e != nil {
		return e
	}
	if _, e = stack.ReviewInstancePlan(spec, args[3]); e != nil {
		return e
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	real, e := filepath.EvalSymlinks(base)
	if e != nil || !filepath.IsAbs(base) || filepath.Clean(base) != base || real != base {
		return errors.New("private runtime base required")
	}
	bin, e := exec.LookPath(args[1])
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
	i, e := runtime.Inventory(ctx)
	if e != nil {
		return e
	}
	r, e := stack.ObserveInstances(spec, args[3], i)
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
