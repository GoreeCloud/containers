package bundle

import (
	"bytes"
	"debug/elf"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// validateGuestELFInterpreter checks the PT_INTERP path for ELF guests before
// journaling a create. This is not a full shared-library or ABI verification.
// Non-ELF entrypoints retain the existing preflight behavior.
func validateGuestELFInterpreter(guest *os.Root, executable string) error {
	file, err := guest.Open(executable)
	if err != nil {
		return errors.New("OCI guest executable cannot be opened")
	}
	defer file.Close()
	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return nil // too short to be ELF; guest execution remains runtime-gated
	}
	if !bytes.Equal(magic[:], []byte{0x7f, 'E', 'L', 'F'}) {
		return nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return errors.New("OCI guest ELF cannot be inspected")
	}
	parsed, err := elf.NewFile(file)
	if err != nil {
		return errors.New("OCI guest ELF header is invalid")
	}
	defer parsed.Close()
	found := false
	for _, segment := range parsed.Progs {
		if segment.Type != elf.PT_INTERP {
			continue
		}
		if found {
			return errors.New("OCI guest ELF declares multiple interpreters")
		}
		found = true
		payload, err := io.ReadAll(io.LimitReader(segment.Open(), 4097))
		if err != nil || len(payload) < 2 || len(payload) > 4096 ||
			payload[len(payload)-1] != 0 || bytes.IndexByte(payload[:len(payload)-1], 0) >= 0 {
			return errors.New("OCI guest ELF interpreter path is invalid")
		}
		interpreter := string(payload[:len(payload)-1])
		if !filepath.IsAbs(interpreter) || filepath.Clean(interpreter) != interpreter {
			return errors.New("OCI guest ELF interpreter must be an absolute clean path")
		}
		info, err := guest.Stat(strings.TrimPrefix(interpreter, "/"))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return errors.New("OCI guest ELF interpreter missing or not executable")
		}
	}
	return nil
}
