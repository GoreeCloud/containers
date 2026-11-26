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

func delegatedObserve(args []string, out io.Writer) error {
	if len(args) != 3 || args[0] != "runc" {
		return errors.New("usage: delegated-observe runc ID ABS_BUNDLE")
	}
	if !filepath.IsAbs(args[2]) || filepath.Clean(args[2]) != args[2] {
		return errors.New("absolute reviewed bundle required")
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	real, e := filepath.EvalSymlinks(base)
	if e != nil || !filepath.IsAbs(base) || filepath.Clean(base) != base || real != base {
		return errors.New("private runtime base required")
	}
	bin, e := exec.LookPath("runc")
	if e != nil {
		return e
	}
	bin, e = filepath.Abs(bin)
	if e != nil {
		return e
	}
	rt, e := engine.New(bin, filepath.Join(base, "goreecloud-containers"), nil)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, e := rt.ObserveDelegated(ctx, args[1], args[2])
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
