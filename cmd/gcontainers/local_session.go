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

func localSession(action string, args []string, out io.Writer) error {
	bin, e := exec.LookPath("runc")
	if e != nil {
		return e
	}
	bin, e = filepath.Abs(bin)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var report any
	switch action {
	case "start-local":
		if len(args) != 7 || args[6] != "--experimental" {
			return errors.New("usage: stack start-local SPEC ADMISSION ADMISSION_SHA EXISTING_STORE PRIVATE_PARENT PRIVATE_LEASE_STORE --experimental")
		}
		s, err := stack.Load(args[0])
		if err != nil {
			return err
		}
		report, e = stack.StartLocal(ctx, s, args[1], args[2], args[3], args[4], bin, args[5])
	case "review-prelaunch", "cancel-prelaunch":
		apply := action == "cancel-prelaunch"
		if (!apply && len(args) != 2) || (apply && (len(args) != 3 || args[2] != "--experimental")) {
			return errors.New("usage: stack review-prelaunch SESSION PRELAUNCH_SHA | stack cancel-prelaunch SESSION PRELAUNCH_SHA --experimental")
		}
		report, e = stack.RecoverPrelaunch(ctx, args[0], args[1], bin, apply)
	case "status-local", "stop-local", "resume-stop":
		stop := action != "status-local"
		if !stop && len(args) != 2 || stop && (len(args) != 3 || args[2] != "--experimental") {
			return errors.New("usage: stack status-local SESSION RECEIPT_SHA | stack stop-local SESSION RECEIPT_SHA --experimental")
		}
		if action == "resume-stop" {
			report, e = stack.ResumeLocalStop(ctx, args[0], args[1], bin)
		} else {
			report, e = stack.ManageLocal(ctx, args[0], args[1], bin, stop)
		}
	default:
		return errors.New("unknown local session action")
	}
	if err := json.NewEncoder(out).Encode(report); err != nil {
		return errors.Join(e, err)
	}
	return e
}
