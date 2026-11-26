package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/stack"
	"io"
	"os/exec"
	"path/filepath"
	"time"
)

func sessionAudit(args []string, out io.Writer) error {
	if len(args) != 4 {
		return errors.New("usage: stack audit-session SPEC ADMISSION ADMISSION_SHA SESSION")
	}
	s, e := stack.Load(args[0])
	if e != nil {
		return e
	}
	b, e := exec.LookPath("runc")
	if e != nil {
		return e
	}
	b, e = filepath.Abs(b)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, e := stack.AuditSession(ctx, s, args[1], args[2], args[3], b)
	if e != nil {
		return e
	}
	if e = json.NewEncoder(out).Encode(r); e != nil {
		return e
	}
	if r.RecoveryReviewRequired {
		return errors.New("session recovery review required")
	}
	return nil
}
