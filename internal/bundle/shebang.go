package bundle

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// inspectGuestShebang reads at most 513 bytes from one anchored entrypoint.
// It does not evaluate command arguments or resolve /usr/bin/env secondary
// commands. All returned interpreter paths are clean and guest-absolute.
func inspectGuestShebang(guest *os.Root, executable string) (string, error) {
	f, err := guest.Open(executable)
	if err != nil {
		return "", errors.New("OCI guest script cannot be inspected")
	}
	defer f.Close()
	header, err := io.ReadAll(io.LimitReader(f, 513))
	if err != nil {
		return "", errors.New("OCI guest script header unreadable")
	}
	if !bytes.HasPrefix(header, []byte("#!")) {
		return "", nil
	}
	end := bytes.IndexByte(header, '\n')
	if end < 0 {
		if len(header) > 256 {
			return "", errors.New("OCI guest shebang header exceeds 256 bytes")
		}
		end = len(header)
	}
	if end > 256 {
		return "", errors.New("OCI guest shebang header exceeds 256 bytes")
	}
	directive := string(header[2:end])
	if strings.ContainsAny(directive, "\x00\r") {
		return "", errors.New("OCI guest shebang header contains prohibited control bytes")
	}
	parts := strings.Fields(directive)
	if len(parts) == 0 {
		return "", errors.New("OCI guest shebang interpreter is missing")
	}
	path := parts[0]
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("OCI guest shebang interpreter path is unsafe")
	}
	return path, nil
}

// validateGuestScriptInterpreter preserves the standalone shallow preflight
// for unit tests and callers; validateGuestEntrypoint verifies whole chains.
func validateGuestScriptInterpreter(guest *os.Root, executable string) error {
	path, err := inspectGuestShebang(guest, executable)
	if err != nil || path == "" {
		return err
	}
	info, err := guest.Stat(strings.TrimPrefix(path, "/"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return errors.New("OCI guest shebang interpreter missing or not executable")
	}
	return nil
}

// validateGuestEntrypoint performs a bounded non-executing chain preflight.
// Linux script interpreters may themselves be scripts; simply checking their
// initial filesystem entry can miss a second missing interpreter. The chain is
// limited to four files and rejects cycles. This is not an immutable snapshot,
// shell/ABI verification, syscall policy enforcement, or runtime acceptance.
func validateGuestEntrypoint(guest *os.Root, executable string) error {
	seen := map[string]bool{}
	current := executable
	for hop := 0; hop < 4; hop++ {
		if seen[current] {
			return errors.New("OCI guest interpreter chain contains a cycle")
		}
		seen[current] = true
		info, err := guest.Stat(current)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return errors.New("OCI guest interpreter missing or not executable")
		}
		if err := validateGuestELFInterpreter(guest, current); err != nil {
			return err
		}
		path, err := inspectGuestShebang(guest, current)
		if err != nil {
			return err
		}
		if path == "" {
			return nil
		}
		current = strings.TrimPrefix(path, "/")
	}
	return errors.New("OCI guest interpreter chain exceeds safe depth")
}
