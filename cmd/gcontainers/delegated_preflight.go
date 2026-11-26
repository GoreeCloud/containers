package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/bundle"
	"io"
	"time"
)

func delegatedPreflight(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: preflight-delegated ABS_BUNDLE")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, e := bundle.PreflightDelegated(ctx, args[0])
	if err := json.NewEncoder(out).Encode(r); err != nil {
		return err
	}
	return e
}
