package bundle

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// validateGuestScriptInterpreter checks the primary shebang interpreter inside
// the confined guest rootfs. It cannot resolve env-style secondary commands,
// validate interpreter dependencies, or prevent rootfs mutation before create.
func validateGuestScriptInterpreter(guest *os.Root, executable string) error {
	f, err := guest.Open(executable)
	if err != nil {
		return errors.New("OCI guest script cannot be inspected")
	}
	defer f.Close()
	header, err := io.ReadAll(io.LimitReader(f, 513))
	if err != nil {
		return errors.New("OCI guest script header unreadable")
	}
	if !bytes.HasPrefix(header, []byte("#!")) {
		return nil
	}
	end := bytes.IndexByte(header, '\n')
	if end < 0 {
		if len(header) > 256 {
			return errors.New("OCI guest shebang header exceeds 256 bytes")
		}
		end = len(header)
	}
	if end > 256 {
		return errors.New("OCI guest shebang header exceeds 256 bytes")
	}
	directive := string(header[2:end])
	if strings.ContainsAny(directive, "\x00\r") {
		return errors.New("OCI guest shebang header contains prohibited control bytes")
	}
	parts := strings.Fields(directive)
	if len(parts) == 0 {
		return errors.New("OCI guest shebang interpreter is missing")
	}
	path := parts[0]
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("OCI guest shebang interpreter path is unsafe")
	}
	info, err := guest.Stat(strings.TrimPrefix(path, "/"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return errors.New("OCI guest shebang interpreter missing or not executable")
	}
	return nil
}
