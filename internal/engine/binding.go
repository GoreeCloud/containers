package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// InstanceBinding identifies observed local metadata and an observed process
// generation. It is private operational evidence, not authenticated ownership.
type InstanceBinding struct {
	ID             string `json:"id"`
	Bundle         string `json:"bundle"`
	MetadataDevice uint64 `json:"metadataDevice"`
	MetadataInode  uint64 `json:"metadataInode"`
	ProcessSHA256  string `json:"processSHA256"`
}

func procGeneration(raw string, pid int64) (string, error) {
	// comm can contain spaces and parentheses. Last ')' precedes state.
	end := strings.LastIndex(raw, ")")
	if end < 0 || !strings.HasPrefix(raw, strconv.FormatInt(pid, 10)+" (") {
		return "", errors.New("invalid process generation")
	}
	fields := strings.Fields(raw[end+1:])
	if len(fields) < 20 {
		return "", errors.New("incomplete process generation")
	}
	start, e := strconv.ParseUint(fields[19], 10, 64)
	if e != nil || start == 0 {
		return "", errors.New("invalid process start time")
	}
	return strconv.FormatUint(start, 10), nil
}
func (e *Engine) boundState(ctx context.Context, id, dir string) (RuntimeState, InstanceBinding, error) {
	var empty RuntimeState
	var b InstanceBinding
	if filepath.Base(e.Binary) != "runc" || !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return empty, b, errors.New("reviewed runc bundle required")
	}
	root, err := os.OpenRoot(e.StateRoot)
	if err != nil {
		return empty, b, err
	}
	defer root.Close()
	before, err := root.Lstat(id)
	if err != nil || privateStateInfo(before) != nil {
		return empty, b, errors.New("private runtime metadata unavailable")
	}
	raw, err := e.call(ctx, "state", id)
	if err != nil {
		return empty, b, err
	}
	s, err := DecodeRuntimeState(raw, id)
	if err != nil {
		return empty, b, err
	}
	var fields struct {
		Bundle string `json:"bundle"`
	}
	if err = json.Unmarshal(raw, &fields); err != nil || fields.Bundle != dir {
		return empty, b, errors.New("runtime bundle path mismatch")
	}
	st, ok := before.Sys().(*syscall.Stat_t)
	if !ok {
		return empty, b, errors.New("metadata identity unavailable")
	}
	b = InstanceBinding{ID: id, Bundle: dir, MetadataDevice: uint64(st.Dev), MetadataInode: st.Ino}
	if s.Status == "running" {
		proc, err := os.OpenRoot("/proc/" + strconv.FormatInt(s.PID, 10))
		if err != nil {
			return empty, InstanceBinding{}, err
		}
		defer proc.Close()
		boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
		if err != nil || len(boot) > 128 {
			return empty, InstanceBinding{}, errors.New("boot identity unavailable")
		}
		first, err := boundedKernelRead(proc, "stat")
		if err != nil {
			return empty, InstanceBinding{}, err
		}
		gen, err := procGeneration(first, s.PID)
		if err != nil {
			return empty, InstanceBinding{}, err
		}
		again, err := e.readState(ctx, id)
		if err != nil || again != s {
			return empty, InstanceBinding{}, errors.New("runtime generation changed")
		}
		last, err := boundedKernelRead(proc, "stat")
		if err != nil {
			return empty, InstanceBinding{}, err
		}
		gen2, err := procGeneration(last, s.PID)
		if err != nil || gen != gen2 {
			return empty, InstanceBinding{}, errors.New("process generation changed")
		}
		b.ProcessSHA256 = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("goreecloud-process-v1\x00"+strings.TrimSpace(string(boot))+"\x00"+strconv.FormatInt(s.PID, 10)+"\x00"+gen)))
	} else if s.Status != "stopped" || s.PID != 0 {
		return empty, InstanceBinding{}, errors.New("binding requires running or stopped state")
	}
	after, err := root.Lstat(id)
	if err != nil || !os.SameFile(before, after) || privateStateInfo(after) != nil {
		return empty, InstanceBinding{}, errors.New("runtime metadata identity changed")
	}
	return s, b, ctx.Err()
}
func (e *Engine) CaptureBinding(ctx context.Context, id, dir string) (InstanceBinding, error) {
	var b InstanceBinding
	err := e.mutate(ctx, id, func() error {
		s, observed, err := e.boundState(ctx, id, dir)
		if err != nil {
			return err
		}
		if s.Status != "running" {
			return errors.New("running binding required")
		}
		b = observed
		return nil
	})
	if err != nil {
		return InstanceBinding{}, err
	}
	return b, nil
}
func sameBinding(want, got InstanceBinding, state string) bool {
	return want.ID == got.ID && want.Bundle == got.Bundle && want.MetadataDevice == got.MetadataDevice && want.MetadataInode == got.MetadataInode && want.MetadataInode != 0 && strings.HasPrefix(want.ProcessSHA256, "sha256:") && len(want.ProcessSHA256) == 71 && (state == "stopped" && got.ProcessSHA256 == "" || state == "running" && want.ProcessSHA256 == got.ProcessSHA256)
}
func (e *Engine) CheckBinding(ctx context.Context, want InstanceBinding) (string, error) {
	var status string
	err := e.mutate(ctx, want.ID, func() error {
		s, b, err := e.boundState(ctx, want.ID, want.Bundle)
		if err != nil {
			return err
		}
		if !sameBinding(want, b, s.Status) {
			return errors.New("reviewed runtime binding changed")
		}
		status = s.Status
		return nil
	})
	if err != nil {
		return "", err
	}
	return status, nil
}

// StopBound rechecks generation under the same cooperative ID lock as the
// signal. A naturally stopped instance needs no signal.
func (e *Engine) StopBound(ctx context.Context, want InstanceBinding) error {
	return e.mutate(ctx, want.ID, func() error {
		s, b, err := e.boundState(ctx, want.ID, want.Bundle)
		if err != nil {
			return err
		}
		if !sameBinding(want, b, s.Status) {
			return errors.New("reviewed runtime binding changed")
		}
		if s.Status == "stopped" {
			return nil
		}
		_, err = e.call(ctx, "kill", want.ID, "SIGTERM")
		return err
	})
}
func (e *Engine) DeleteBound(ctx context.Context, want InstanceBinding) error {
	return e.mutate(ctx, want.ID, func() error {
		s, b, err := e.boundState(ctx, want.ID, want.Bundle)
		if err != nil {
			return err
		}
		if s.Status != "stopped" || !sameBinding(want, b, s.Status) {
			return errors.New("stopped reviewed binding required")
		}
		_, err = e.call(ctx, "delete", want.ID)
		return err
	})
}
