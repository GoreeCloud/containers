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

func storeManifestAssembly(args []string, out io.Writer) error {
	if len(args) != 5 || args[4] != "--experimental" {
		return errors.New("usage: image assemble-manifest EXISTING_STORE SHA256 SIZE PRIVATE_PARENT --experimental")
	}
	n, e := strconv.ParseInt(args[2], 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != args[2] {
		return errors.New("canonical descriptor size required")
	}
	s, e := ociimage.OpenExistingStore(args[0])
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r, e := s.AssembleManifest(ctx, args[1], n, args[3])
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
