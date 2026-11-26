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

func pullStage(args []string, out io.Writer) error {
	if len(args) != 7 || args[6] != "--experimental" {
		return errors.New("usage: image pull-stage EXISTING_STORE HTTPS_ORIGIN REPOSITORY SHA256 SIZE PRIVATE_PARENT --experimental")
	}
	n, e := strconv.ParseInt(args[4], 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != args[4] {
		return errors.New("canonical manifest size required")
	}
	s, e := ociimage.OpenExistingStore(args[0])
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	report, e := s.PullAndStage(ctx, args[1], args[2], args[3], n, args[5])
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(report)
}
