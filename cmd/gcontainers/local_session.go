package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/stack"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
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
	case "watch-local":
		if len(args) != 4 {
			return errors.New("usage: stack watch-local SESSION RECEIPT_SHA SAMPLES INTERVAL_SECONDS")
		}
		samples, err := strconv.Atoi(args[2])
		if err != nil {
			return errors.New("invalid sample count")
		}
		seconds, err := strconv.Atoi(args[3])
		if err != nil || seconds < 1 || seconds > 10 {
			return errors.New("invalid interval")
		}
		watchCtx, stopWatch := context.WithTimeout(context.Background(), 6*time.Minute)
		defer stopWatch()
		return stack.WatchLocal(watchCtx, args[0], args[1], bin, samples, time.Duration(seconds)*time.Second, func(s stack.LocalWatchSample) error { return json.NewEncoder(out).Encode(s) })
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
	case "review-start", "finalize-start":
		apply := action == "finalize-start"
		if (!apply && len(args) != 2) || (apply && (len(args) != 3 || args[2] != "--experimental")) {
			return errors.New("usage: stack review-start SESSION RECEIPT_SHA | stack finalize-start SESSION RECEIPT_SHA --experimental")
		}
		report, e = stack.FinalizeLocalStart(ctx, args[0], args[1], bin, apply)
	case "review-stop":
		if len(args) != 2 {
			return errors.New("usage: stack review-stop SESSION RECEIPT_SHA")
		}
		report, e = stack.ReviewLocalStop(ctx, args[0], args[1], bin)
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
