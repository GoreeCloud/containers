package volume

import (
	"context"
	"errors"
	"os"
	"syscall"
)

// Fingerprint produces the same bounded content digest as backup/restore,
// without allocating a backup or altering the selected volume.
func Fingerprint(ctx context.Context, store, id, pin string) (Verification, error) {
	var empty Verification
	if ctx == nil || !validID.MatchString(id) || (pin != "" && !validPin(pin)) {
		return empty, errors.New("invalid volume or digest")
	}
	r, e := open(store)
	if e != nil {
		return empty, e
	}
	defer r.Close()
	lock, e := lockStore(ctx, r, syscall.LOCK_SH)
	if e != nil {
		return empty, e
	}
	defer lock.Close()
	parent, e := r.Stat(".")
	if e != nil {
		return empty, e
	}
	before, e := r.Lstat(id)
	if e != nil || !privateDir(before) {
		return empty, errors.New("invalid source volume")
	}
	src, e := r.OpenRoot(id)
	if e != nil {
		return empty, e
	}
	defer src.Close()
	opened, e := src.Stat(".")
	if e != nil || !samePayload(before, opened) {
		return empty, errors.New("volume identity changed")
	}
	rm, e := rootMount(r)
	if e != nil {
		return empty, e
	}
	sm, e := rootMount(src)
	if e != nil || rm != sm {
		return empty, errors.New("source volume crosses mount boundary")
	}
	a, e := copyPayload(ctx, src, nil)
	if e != nil {
		return empty, e
	}
	b, e := copyPayload(ctx, src, nil)
	if e != nil || a != b {
		return empty, errors.New("volume content changed between observations")
	}
	current, e := r.Lstat(id)
	if e != nil || !samePayload(before, current) {
		return empty, errors.New("source volume changed")
	}
	current, e = os.Lstat(store)
	if e != nil || !privateDir(current) || !os.SameFile(parent, current) {
		return empty, errors.New("volume store changed")
	}
	if e = ctx.Err(); e != nil {
		return empty, e
	}
	if pin != "" && a.ContentSHA256 != pin {
		return empty, errors.New("volume content pin mismatch")
	}
	return Verification{ReadOnly: true, ContentSHA256: a.ContentSHA256, DigestMatches: pin != "", Files: a.Files, Bytes: a.Bytes}, nil
}
