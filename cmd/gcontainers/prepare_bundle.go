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

func prepareImageBundle(args []string, out io.Writer) error {
	if len(args) != 8 || args[7] != "--experimental" {
		return errors.New("usage: image prepare-bundle EXISTING_STORE MANIFEST_SHA SIZE CONFIG_FILE CONFIG_SHA ENTRYPOINT_SHA PRIVATE_PARENT --experimental")
	}
	size, e := strconv.ParseInt(args[2], 10, 64)
	if e != nil || size <= 0 || strconv.FormatInt(size, 10) != args[2] {
		return errors.New("canonical manifest size required")
	}
	store, e := ociimage.OpenExistingStore(args[0])
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r, e := store.PrepareBundle(ctx, args[1], size, args[3], args[4], args[5], args[6])
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
