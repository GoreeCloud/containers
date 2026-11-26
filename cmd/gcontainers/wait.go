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
	"strconv"
	"time"
)

func waitCommand(args []string, out io.Writer) error {
	if len(args) != 3 || (args[0] != "runc" && args[0] != "crun") {
		return errors.New("usage: wait runc|crun ID SECONDS")
	}
	n, e := strconv.Atoi(args[2])
	if e != nil || n < 1 || n > 60 || strconv.Itoa(n) != args[2] {
		return errors.New("canonical wait duration 1–60 seconds required")
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	real, e := filepath.EvalSymlinks(base)
	if e != nil || !filepath.IsAbs(base) || filepath.Clean(base) != base || real != base {
		return errors.New("private runtime directory required")
	}
	binary, e := exec.LookPath(args[0])
	if e != nil {
		return e
	}
	binary, e = filepath.Abs(binary)
	if e != nil {
		return e
	}
	runtime, e := engine.New(binary, filepath.Join(base, "goreecloud-containers"), nil)
	if e != nil {
		return e
	}
	r, e := runtime.WaitStopped(context.Background(), args[1], time.Duration(n)*time.Second)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
