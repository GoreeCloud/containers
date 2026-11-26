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
	if len(args) < 7 || len(args) > 10 || args[len(args)-1] != "--experimental" {
		return errors.New("usage: image pull-stage EXISTING_STORE HTTPS_ORIGIN REPOSITORY SHA256 SIZE PRIVATE_PARENT [--bearer-file PRIVATE_TOKEN_FILE] [--whiteouts] --experimental")
	}
	options := args[6 : len(args)-1]
	token := ""
	if len(options) >= 2 && options[0] == "--bearer-file" {
		token = options[1]
		if token == "" {
			return errors.New("private bearer file required")
		}
		options = options[2:]
	}
	whiteouts := len(options) == 1 && options[0] == "--whiteouts"
	if len(options) != 0 && !whiteouts {
		return errors.New("unsupported pull-stage options")
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
	var report ociimage.PullStageReport
	if token != "" {
		report, e = s.PullAndStageAuthenticatedWithPolicy(ctx, args[1], args[2], args[3], n, args[5], token, whiteouts)
	} else {
		report, e = s.PullAndStageWithPolicy(ctx, args[1], args[2], args[3], n, args[5], whiteouts)
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(report)
}
