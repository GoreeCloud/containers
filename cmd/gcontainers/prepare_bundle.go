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
	if len(args) < 8 || len(args) > 10 || args[len(args)-1] != "--experimental" {
		return errors.New("usage: image prepare-bundle EXISTING_STORE MANIFEST_SHA SIZE CONFIG_FILE CONFIG_SHA ENTRYPOINT_SHA PRIVATE_PARENT [--whiteouts] [--delegated-guest] --experimental")
	}
	opts := args[7 : len(args)-1]
	whiteouts, delegated := false, false
	for _, o := range opts {
		switch o {
		case "--whiteouts":
			if whiteouts {
				return errors.New("duplicate whiteout option")
			}
			whiteouts = true
		case "--delegated-guest":
			if delegated {
				return errors.New("duplicate delegated option")
			}
			delegated = true
		default:
			return errors.New("unsupported preparation option")
		}
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
	var r ociimage.PreparedBundle
	if delegated {
		r, e = store.PrepareDelegatedBundle(ctx, args[1], size, args[3], args[4], args[5], args[6], whiteouts)
	} else if whiteouts {
		r, e = store.PrepareBundleWithWhiteouts(ctx, args[1], size, args[3], args[4], args[5], args[6])
	} else {
		r, e = store.PrepareBundle(ctx, args[1], size, args[3], args[4], args[5], args[6])
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
