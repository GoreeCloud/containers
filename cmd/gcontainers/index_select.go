package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/ociimage"
	"io"
	"strconv"
	"time"
)

func selectImageIndex(args []string, out io.Writer) error {
	if len(args) != 4 {
		return errors.New("usage: image select-index EXISTING_STORE INDEX_SHA SIZE amd64|arm64")
	}
	n, e := strconv.ParseInt(args[2], 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != args[2] {
		return errors.New("canonical index size required")
	}
	s, e := ociimage.OpenExistingStore(args[0])
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, e := s.SelectIndex(ctx, args[1], n, args[3])
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
