package main

import (
	"context"
	"errors"
	"github.com/GoreeCloud/containers/internal/localapi"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func localAPI(args []string) error {
	if len(args) != 2 || args[1] != "--experimental" {
		return errors.New("usage: serve-local ABS_SOCKET --experimental")
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return errors.New("private XDG_RUNTIME_DIR required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	timed, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	return localapi.Serve(timed, args[0], filepath.Join(base, "goreecloud-containers"))
}
