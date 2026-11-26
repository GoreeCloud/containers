package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// A runtime success code alone is not deletion evidence. Keep the journal
// until the exact runtime metadata directory is absent under the original
// anchored state root. Failed state queries are not evidence of absence.
func captureDeleteRoot(stateRoot, id string) (*os.Root, os.FileInfo, error) {
	if e := ValidateID(id); e != nil {
		return nil, nil, e
	}
	if id == "locks" || id == "journal" {
		return nil, nil, errors.New("reserved runtime ID")
	}
	r, e := journalRoot(stateRoot)
	if e != nil {
		return nil, nil, e
	}
	fail := func(e error) (*os.Root, os.FileInfo, error) { r.Close(); return nil, nil, e }
	before, e := r.Lstat(".")
	if e != nil {
		return fail(e)
	}
	current, e := os.Lstat(stateRoot)
	if e != nil || !os.SameFile(before, current) {
		return fail(errors.New("runtime state root changed"))
	}
	meta, e := r.Lstat(id)
	if e != nil || !meta.IsDir() || privateStateInfo(meta) != nil {
		return fail(errors.New("runtime metadata missing or unsafe before deletion"))
	}
	return r, before, nil
}
func verifyDeleteAbsent(ctx context.Context, r *os.Root, before os.FileInfo, stateRoot, id string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	current, e := os.Lstat(filepath.Clean(stateRoot))
	if e != nil || !os.SameFile(before, current) || privateStateInfo(current) != nil {
		return errors.New("runtime root changed during deletion")
	}
	if _, e = r.Lstat(id); !os.IsNotExist(e) {
		return errors.New("post-delete runtime metadata absence not verified")
	}
	return ctx.Err()
}
