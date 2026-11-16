package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RuntimeSecurityBaseline is a deliberately conservative upstream-version
// preflight reviewed on 2026-11-16. It does not replace vendor advisories,
// provenance checks, signatures, or security review of the host.
func RuntimeSecurityBaseline(backend string) string {
	switch backend {
	case "crun":
		return "1.31.0"
	case "runc":
		return "1.5.3"
	default:
		return ""
	}
}

// CheckVersionOutput accepts only a stable numeric upstream release from the
// expected runtime; aliases, release candidates and unclear output fail closed.
func CheckVersionOutput(backend, output string) error {
	minimum := RuntimeSecurityBaseline(backend)
	if minimum == "" {
		return errors.New("unsupported OCI runtime backend")
	}
	firstLine, _, _ := strings.Cut(output, "\n")
	fields := strings.Fields(firstLine)
	if len(fields) < 3 || fields[0] != backend || fields[1] != "version" {
		return errors.New("unable to verify OCI runtime version")
	}
	value := strings.TrimPrefix(fields[2], "v")
	if backend == "crun" && strings.Count(value, ".") == 1 {
		value += ".0"
	}
	actual, err := versionTriple(value)
	if err != nil {
		return errors.New("OCI runtime version must be an unambiguous stable release")
	}
	target, _ := versionTriple(minimum)
	for i := range actual {
		if actual[i] < target[i] {
			return fmt.Errorf("%s runtime is below the minimum reviewed upstream version %s", backend, minimum)
		}
		if actual[i] > target[i] {
			return nil
		}
	}
	return nil
}
func versionTriple(value string) ([3]uint64, error) {
	var version [3]uint64
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return version, errors.New("not a three-component stable version")
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version, errors.New("invalid version component")
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return version, errors.New("non-numeric version component")
			}
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version, err
		}
		version[i] = parsed
	}
	return version, nil
}

// CheckInstalledRuntime runs only a trusted, root-owned system executable
// with --version and a short deadline. Its output is bounded by ProcessRunner.
func CheckInstalledRuntime(ctx context.Context, binary string) error {
	if err := TrustedRuntimeBinary(binary); err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("context required for runtime-version check")
	}
	timed, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := (ProcessRunner{}).Run(timed, binary, "--version")
	if err != nil {
		return errors.New("OCI runtime version probe failed")
	}
	return CheckVersionOutput(filepath.Base(binary), string(output))
}
