package main

import (
	"context"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"github.com/GoreeCloud/containers/internal/localapi"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func localAPI(args []string) error {
	if (len(args) != 2 || args[1] != "--experimental") && (len(args) != 4 || args[1] != "--runtime" || (args[2] != "runc" && args[2] != "crun") || args[3] != "--experimental") {
		return errors.New("usage: serve-local ABS_SOCKET [--runtime runc|crun] --experimental")
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return errors.New("private XDG_RUNTIME_DIR required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	timed, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	root := filepath.Join(base, "goreecloud-containers")
	if len(args) == 4 {
		real, e := filepath.EvalSymlinks(base)
		if e != nil || real != base {
			return errors.New("unsafe runtime base")
		}
		binary, e := exec.LookPath(args[2])
		if e != nil {
			return e
		}
		binary, e = filepath.Abs(binary)
		if e != nil {
			return e
		}
		runner, e := engine.New(binary, root, nil)
		if e != nil {
			return e
		}
		return localapi.ServeRuntime(timed, args[0], root, runner)
	}
	return localapi.Serve(timed, args[0], root)
}
