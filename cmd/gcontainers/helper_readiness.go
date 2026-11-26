package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/hostreadiness"
	"io"
	"time"
)

func helperReadiness(args []string, out io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: host helpers")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, e := hostreadiness.InspectHelpers(ctx)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
