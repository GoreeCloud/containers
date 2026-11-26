package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/bundle"
	"io"
	"time"
)

func bundleSnapshot(args []string, out io.Writer) error    { return snapshotCommand(args, out, false) }
func delegatedSnapshot(args []string, out io.Writer) error { return snapshotCommand(args, out, true) }
func snapshotCommand(args []string, out io.Writer, delegated bool) error {
	if len(args) != 3 || args[2] != "--experimental" {
		return errors.New("usage: gcontainers snapshot ABS_BUNDLE PRIVATE_PARENT --experimental")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var report bundle.SnapshotReport
	var err error
	if delegated {
		report, err = bundle.SnapshotDelegated(ctx, args[0], args[1])
	} else {
		report, err = bundle.Snapshot(ctx, args[0], args[1])
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(report)
}
