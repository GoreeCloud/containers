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

func registryBlob(args []string, out io.Writer) error {
	if len(args) != 6 || args[5] != "--experimental" {
		return errors.New("usage: image fetch-blob EXISTING_STORE HTTPS_ORIGIN REPOSITORY SHA256 SIZE --experimental")
	}
	n, e := strconv.ParseInt(args[4], 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != args[4] {
		return errors.New("canonical descriptor size required")
	}
	s, e := ociimage.OpenExistingStore(args[0])
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, e := s.FetchRegistryBlob(ctx, args[1], args[2], args[3], n)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(map[string]any{"digest": info.Digest, "size": info.Size, "digestMatches": true, "sourceProvenanceVerified": false, "executionAuthorized": false})
}
