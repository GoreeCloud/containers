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

func watchCommand(args []string, out io.Writer) error {
	if len(args) != 3 || (args[0] != "runc" && args[0] != "crun") {
		return errors.New("usage: watch runc|crun SAMPLES INTERVAL_SECONDS")
	}
	n, e := strconv.Atoi(args[1])
	if e != nil || n < 1 || n > 60 || strconv.Itoa(n) != args[1] {
		return errors.New("canonical sample count required")
	}
	sec, e := strconv.Atoi(args[2])
	if e != nil || sec < 1 || sec > 10 || strconv.Itoa(sec) != args[2] {
		return errors.New("canonical interval required")
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
	r, e := engine.New(bin, filepath.Join(base, "goreecloud-containers"), nil)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(n*(sec+5)+1)*time.Second)
	defer cancel()
	enc := json.NewEncoder(out)
	return r.Watch(ctx, n, time.Duration(sec)*time.Second, func(s engine.WatchSample) error { return enc.Encode(s) })
}
