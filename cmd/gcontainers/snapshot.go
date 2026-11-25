package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/bundle"
	"io"
	"time"
)

func bundleSnapshot(args []string, out io.Writer) error {
	if len(args) != 3 || args[2] != "--experimental" {
		return errors.New("usage: gcontainers snapshot ABS_BUNDLE PRIVATE_PARENT --experimental")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := bundle.Snapshot(ctx, args[0], args[1])
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(report)
}
