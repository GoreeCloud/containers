package volume

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
)

type Verification struct {
	ReadOnly              bool   `json:"readOnly"`
	ContentSHA256         string `json:"contentSHA256"`
	DigestMatches         bool   `json:"digestMatches"`
	Files                 int    `json:"files"`
	Bytes                 int64  `json:"bytes"`
	ApplicationConsistent bool   `json:"applicationConsistent"`
	MountVerified         bool   `json:"mountVerified"`
}

func validPin(d string) bool {
	if len(d) != 71 || !strings.HasPrefix(d, "sha256:") {
		return false
	}
	for _, c := range d[7:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// VerifyBackup never creates a destination or copies payload. Uses the same
// normalized byte stream as Backup/Restore; a digest is not a signature or
// a guarantee of application consistency.
func VerifyBackup(ctx context.Context, backup, pin string) (Verification, error) {
	var empty Verification
	if ctx == nil || !validPin(pin) {
		return empty, errors.New("valid backup digest required")
	}
	if e := ctx.Err(); e != nil {
		return empty, e
	}
	src, e := open(backup)
	if e != nil {
		return empty, e
	}
	defer src.Close()
	guard, e := lockStore(ctx, src, syscall.LOCK_SH)
	if e != nil {
		return empty, e
	}
	defer guard.Close()
	before, e := src.Stat(".")
	if e != nil {
		return empty, e
	}
	r, e := copyPayload(ctx, src, nil)
	if e != nil {
		return empty, e
	}
	current, e := os.Lstat(backup)
	if e != nil || !samePayload(before, current) {
		return empty, errors.New("backup root changed during verification")
	}
	if e = ctx.Err(); e != nil {
		return empty, e
	}
	if r.ContentSHA256 != pin {
		return empty, errors.New("backup content pin mismatch")
	}
	return Verification{ReadOnly: true, ContentSHA256: r.ContentSHA256, DigestMatches: true, Files: r.Files, Bytes: r.Bytes}, nil
}
