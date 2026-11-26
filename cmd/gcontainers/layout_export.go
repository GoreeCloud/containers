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

func layoutExport(args []string, out io.Writer) error {
	if (len(args) != 5 || args[4] != "--experimental") && (len(args) != 6 || args[4] != "--whiteouts" || args[5] != "--experimental") {
		return errors.New("usage: image export-layout EXISTING_STORE MANIFEST_SHA SIZE PRIVATE_PARENT [--whiteouts] --experimental")
	}
	n, e := strconv.ParseInt(args[2], 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != args[2] {
		return errors.New("canonical manifest size required")
	}
	s, e := ociimage.OpenExistingStore(args[0])
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r, e := s.ExportLayout(ctx, args[1], n, args[3], len(args) == 6)
	if err := json.NewEncoder(out).Encode(r); err != nil {
		return errors.Join(e, err)
	}
	return e
}
