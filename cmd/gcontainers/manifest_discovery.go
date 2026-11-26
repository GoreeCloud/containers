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

func manifestDiscovery(args []string, out io.Writer) error {
	if len(args) != 5 || args[4] != "--experimental" {
		return errors.New("usage: image discover-manifest HTTPS_ORIGIN REPOSITORY SHA256 SIZE --experimental")
	}
	size, e := strconv.ParseInt(args[3], 10, 64)
	if e != nil || size <= 0 || strconv.FormatInt(size, 10) != args[3] {
		return errors.New("canonical descriptor size required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, e := ociimage.DiscoverManifest(ctx, args[0], args[1], args[2], size)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
