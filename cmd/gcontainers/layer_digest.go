package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/GoreeCloud/containers/internal/ociimage"
)

// image verify-layer checks a local tar or tar+gzip against a caller-pinned
// SHA256 descriptor in one read-only parsing pass. No extraction or execution.
func imageVerifyLayer(args []string, out io.Writer) error {
	if len(args) != 3 && len(args) != 4 {
		return errors.New("usage: image verify-layer ABS_FILE tar|tar+gzip sha256:HEX [DESCRIPTOR_BYTES]")
	}
	var expectedSize int64
	if len(args) == 4 {
		if len(args[3]) == 0 || len(args[3]) > 20 || args[3][0] == '0' {
			return errors.New("invalid positive descriptor size")
		}
		for _, digit := range args[3] {
			if digit < '0' || digit > '9' {
				return errors.New("invalid numeric descriptor size")
			}
		}
		var err error
		expectedSize, err = strconv.ParseInt(args[3], 10, 64)
		if err != nil || expectedSize <= 0 || expectedSize > ociimage.MaxLayerCompressedBytes {
			return errors.New("descriptor size outside supported bounds")
		}
	}
	source := args[0]
	if !filepath.IsAbs(source) || filepath.Clean(source) != source {
		return errors.New("expected clean absolute local OCI layer filename")
	}
	parent := filepath.Dir(source)
	real, err := filepath.EvalSymlinks(parent)
	if err != nil || real != parent {
		return errors.New("symlinked layer parent directory unsupported")
	}
	mode := ""
	switch args[1] {
	case "tar":
		mode = ociimage.MediaLayer
	case "tar+gzip":
		mode = ociimage.MediaLayerGzip
	default:
		return errors.New("only tar or tar+gzip layer subset supported")
	}
	before, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > ociimage.MaxLayerCompressedBytes {
		return errors.New("layer must be a bounded, nonsymlink regular file")
	}
	if expectedSize > 0 && before.Size() != expectedSize {
		return errors.New("local OCI layer bytes differ from descriptor")
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return errors.New("layer inode changed before opening")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var report ociimage.VerifiedLayerReport
	if expectedSize > 0 {
		report, err = ociimage.VerifyLayerWithDescriptor(ctx, file, mode, args[2], expectedSize)
	} else {
		report, err = ociimage.VerifyLayerWithDigest(ctx, file, mode, args[2])
	}
	if err != nil {
		return err
	}
	afterFD, err := file.Stat()
	if err != nil {
		return err
	}
	afterPath, err := os.Lstat(source)
	if err != nil || !os.SameFile(opened, afterFD) || !os.SameFile(opened, afterPath) ||
		afterFD.Size() != before.Size() || afterPath.Size() != before.Size() {
		return errors.New("layer changed during digest verification")
	}
	result, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}
