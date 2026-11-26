package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var ErrUnresolvedMutation = errors.New("unresolved OCI mutation exists; operator recovery required")

// PendingMutation is a minimal, read-only incident summary. It is not proof
// that the OCI runtime created or removed any process.
type PendingMutation struct {
	ID               string `json:"id"`
	Operation        string `json:"operation,omitempty"`
	Pending          bool   `json:"pending"`
	RecoveryRequired bool   `json:"recoveryRequired"`
	ReadOnly         bool   `json:"readOnly"`
}

type journalRecord struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Operation string `json:"operation"`
}

func journalRoot(stateRoot string) (*os.Root, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("rootless journal only")
	}
	if !filepath.IsAbs(stateRoot) || filepath.Clean(stateRoot) != stateRoot {
		return nil, errors.New("invalid absolute state root")
	}
	if err := privateDir(stateRoot); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(stateRoot)
	if err != nil {
		return nil, err
	}
	release := func(err error) (*os.Root, error) { root.Close(); return nil, err }
	info, err := root.Lstat(".")
	if err != nil || privateStateInfo(info) != nil {
		return release(errors.New("untrusted runtime root"))
	}
	dir, err := root.Lstat("journal")
	if err != nil || privateStateInfo(dir) != nil {
		return release(errors.New("missing or unsafe OCI journal directory"))
	}
	return root, nil
}
func journalOp(op string) bool {
	switch op {
	case "create", "start", "kill", "delete":
		return true
	}
	return false
}

// RunJournaledMutation MUST be used only inside the cooperative per-ID lock.
// An incomplete or failing operation leaves a durable marker and blocks all
// subsequent mutations, even if the runtime may have completed the change.
func RunJournaledMutation(ctx context.Context, stateRoot, id, op string, action func() ([]byte, error)) ([]byte, error) {
	if ctx == nil || action == nil {
		return nil, errors.New("context and action required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	if !journalOp(op) {
		return nil, errors.New("unsupported journal operation")
	}
	root, err := journalRoot(stateRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := "journal/" + id + ".pending"
	output, err := json.Marshal(journalRecord{Version: 1, ID: id, Operation: op})
	if err != nil {
		return nil, err
	}
	pending, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if os.IsExist(err) {
		return nil, ErrUnresolvedMutation
	}
	if err != nil {
		return nil, fmt.Errorf("cannot create exclusive OCI journal: %w", err)
	}
	// No action is performed until the pending marker is durably synced.
	n, writeErr := pending.Write(output)
	if writeErr == nil && n != len(output) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = pending.Sync()
	}
	markerInfo, statErr := pending.Stat()
	closeErr := pending.Close()
	if writeErr != nil || statErr != nil || closeErr != nil {
		return nil, errors.New("unable to persist OCI mutation marker; recovery review required")
	}
	if err := syncJournalDirectory(root); err != nil {
		return nil, err
	}
	if err := verifyPendingIdentity(root, name, markerInfo); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := action()
	if err != nil {
		return nil, fmt.Errorf("%w: runtime returned uncertain state: %v", ErrUnresolvedMutation, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: operation context ended before completion", ErrUnresolvedMutation)
	}
	if err := verifyPendingIdentity(root, name, markerInfo); err != nil {
		return nil, err
	}
	if err := root.Remove(name); err != nil {
		return nil, fmt.Errorf("OCI operation succeeded but journal cleanup needs recovery: %w", err)
	}
	if err := syncJournalDirectory(root); err != nil {
		return nil, fmt.Errorf("OCI journal completion sync failed: %w", err)
	}
	return result, nil
}

// Detect marker replacement before invoking or committing a mutation.
// This is a cooperative safety check, not atomic compare-and-unlink or
// protection against an actor with the same UID.
func verifyPendingIdentity(root *os.Root, name string, initial os.FileInfo) error {
	current, err := root.Lstat(name)
	if err != nil || initial == nil || !os.SameFile(initial, current) ||
		!current.Mode().IsRegular() || current.Mode().Perm()&0077 != 0 ||
		current.Size() != initial.Size() || current.Mode() != initial.Mode() || !current.ModTime().Equal(initial.ModTime()) {
		return fmt.Errorf("%w: journal marker changed", ErrUnresolvedMutation)
	}
	st, ok := current.Sys().(*syscall.Stat_t)
	before, okBefore := initial.Sys().(*syscall.Stat_t)
	if !ok || !okBefore || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || before.Ctim != st.Ctim {
		return fmt.Errorf("%w: journal marker ownership changed", ErrUnresolvedMutation)
	}
	return nil
}

func syncJournalDirectory(root *os.Root) error {
	dir, err := root.Open("journal")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// ReadPendingMutation inspects only this user's private journal; it does not
// create files, clear markers, contact the OCI runtime or alter workloads.
func ReadPendingMutation(ctx context.Context, stateRoot, id string) (PendingMutation, error) {
	empty := PendingMutation{}
	if ctx == nil {
		return empty, errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := ValidateID(id); err != nil {
		return empty, err
	}
	root, err := journalRoot(stateRoot)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	name := "journal/" + id + ".pending"
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return PendingMutation{ID: id, ReadOnly: true}, nil
	}
	if err != nil {
		return empty, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 512 || info.Mode().Perm()&0077 != 0 {
		return empty, errors.New("unsafe OCI pending marker")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return empty, errors.New("pending marker ownership or hardlink unsafe")
	}
	f, err := regularinput.Open(root, name, info)
	if err != nil {
		return empty, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return empty, errors.New("pending marker changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 513))
	if err != nil || int64(len(raw)) != info.Size() {
		return empty, errors.New("pending marker changed while reading")
	}
	if err := verifyPendingIdentity(root, name, info); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := uniqueRuntimeJSON(raw); err != nil {
		return empty, errors.New("ambiguous pending marker")
	}
	var parsed journalRecord
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parsed); err != nil {
		return empty, errors.New("invalid pending marker")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return empty, errors.New("trailing pending marker data")
	}
	if parsed.Version != 1 || parsed.ID != id || !journalOp(parsed.Operation) {
		return empty, errors.New("inconsistent pending marker")
	}
	return PendingMutation{ID: id, Operation: parsed.Operation, Pending: true, RecoveryRequired: true, ReadOnly: true}, nil
}
