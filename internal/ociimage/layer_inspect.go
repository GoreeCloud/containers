package ociimage

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	MaxLayerCompressedBytes int64 = 128 << 20
	MaxLayerUnpackedBytes   int64 = 256 << 20
	MaxLayerEntries               = 4096
	MaxLayerTrailingZeros   int64 = 64 << 10
)

type LayerReport struct {
	FileCount      int   `json:"fileCount"`
	DirectoryCount int   `json:"directoryCount"`
	UnpackedBytes  int64 `json:"unpackedBytes"`
}

// InspectLayer parses an intentionally strict OCI tar/tar+gzip preview subset.
// It NEVER writes to disk, extracts, applies whiteouts, or executes anything.
// This is NOT a full OCI layer implementation or safe-image certification.
func InspectLayer(ctx context.Context, input io.Reader, mediaType string) (LayerReport, error) {
	var empty LayerReport
	if ctx == nil || input == nil {
		return empty, errors.New("layer inspection requires context and input")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if mediaType != MediaLayer && mediaType != MediaLayerGzip {
		return empty, errors.New("unsupported OCI layer compression or media type")
	}
	source := &boundedLayerReader{ctx: ctx, reader: input, max: MaxLayerCompressedBytes}
	var decoded io.Reader = source
	var compressedRemainder *bufio.Reader
	if mediaType == MediaLayerGzip {
		// gzip.NewReader consumes buffered bytes internally unless given an
		// io.ByteReader. Keep the remaining bytes observable after the first
		// gzip member, and explicitly prohibit concatenated gzip streams.
		compressedRemainder = bufio.NewReaderSize(source, 4096)
		decoder, err := gzip.NewReader(compressedRemainder)
		if err != nil {
			return empty, errors.New("invalid gzip layer header")
		}
		decoder.Multistream(false)
		defer decoder.Close()
		decoded = decoder
	}
	reader := tar.NewReader(&layerContextReader{ctx: ctx, reader: decoded})
	seen := map[string]byte{}
	implied := map[string]bool{}
	var report LayerReport
	for {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return empty, fmt.Errorf("malformed OCI layer tar: %w", err)
		}
		if report.FileCount+report.DirectoryCount >= MaxLayerEntries {
			return empty, errors.New("OCI layer entry limit exceeded")
		}
		if header == nil || header.Size < 0 || header.Size > MaxLayerUnpackedBytes {
			return empty, errors.New("invalid OCI layer entry size")
		}
		if header.Linkname != "" || header.Mode&^int64(0o777) != 0 ||
			header.Uid < 0 || header.Gid < 0 || len(header.PAXRecords) > 0 || len(header.Xattrs) > 0 {
			return empty, errors.New("unsupported OCI entry links, privilege metadata or extended headers")
		}
		name := header.Name
		directory := header.Typeflag == tar.TypeDir
		if directory {
			name = strings.TrimSuffix(name, "/")
		}
		if name == "" || name == "." || len(name) > 255 || strings.Contains(name, "\\") ||
			strings.HasPrefix(name, "/") || !validLayerName(name) {
			return empty, errors.New("unsafe OCI layer entry path")
		}
		if strings.HasPrefix(path.Base(name), ".wh.") {
			return empty, errors.New("OCI whiteout processing is not supported in preview")
		}
		if seen[name] != 0 {
			return empty, errors.New("duplicate OCI layer entry path")
		}
		if directory {
			if header.Size != 0 {
				return empty, errors.New("OCI directory contains payload")
			}
			if header.Typeflag != tar.TypeDir {
				return empty, errors.New("invalid directory entry")
			}
			seen[name] = 'd'
			report.DirectoryCount++
		} else {
			if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
				return empty, errors.New("unsupported OCI layer entry type")
			}
			if implied[name] {
				return empty, errors.New("regular file conflicts with nested paths")
			}
			seen[name] = 'f'
			if header.Size > MaxLayerUnpackedBytes-report.UnpackedBytes {
				return empty, errors.New("OCI layer exceeds total uncompressed size quota")
			}
			n, err := io.CopyN(io.Discard, reader, header.Size)
			if err != nil || n != header.Size {
				return empty, errors.New("truncated OCI layer member")
			}
			report.FileCount++
			report.UnpackedBytes += n
		}
		for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if seen[parent] == 'f' {
				return empty, errors.New("regular file is an OCI path ancestor")
			}
			implied[parent] = true
		}
	}
	// Drain tar padding, detect nonzero suffixes, and verify gzip trailers.
	// GNU/PAX archives, whiteouts and multi-stream OCI extensions remain unsupported.
	trailer := &layerContextReader{ctx: ctx, reader: decoded}
	buffer := make([]byte, 16<<10)
	var padded int64
	for {
		n, err := trailer.Read(buffer)
		padded += int64(n)
		if padded > MaxLayerTrailingZeros {
			return empty, errors.New("OCI tar trailer exceeds preview limit")
		}
		for _, byteValue := range buffer[:n] {
			if byteValue != 0 {
				return empty, errors.New("unexpected nonzero OCI tar trailing data")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return empty, fmt.Errorf("OCI trailer verification failed: %w", err)
		}
	}
	if compressedRemainder != nil {
		// gzip trailer and CRC have been verified by the full decoded drain.
		// Any additional compressed bytes (including a second gzip member)
		// make the intended OCI layer ambiguous and are rejected.
		var sentinel [1]byte
		n, err := compressedRemainder.Read(sentinel[:])
		if n > 0 || err != io.EOF {
			return empty, errors.New("trailing gzip members or compressed data unsupported")
		}
	}
	return report, nil
}
func validLayerName(name string) bool {
	if path.Clean(name) != name || strings.ContainsRune(name, 0) {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.Contains(segment, ":") {
			return false
		}
	}
	return true
}

type layerContextReader struct {
	ctx        context.Context
	reader     io.Reader
	emptyReads int
}

func (r *layerContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if n < 0 || n > len(p) {
		return 0, errors.New("invalid OCI layer reader byte count")
	}
	if n == 0 && err == nil {
		r.emptyReads++
		if r.emptyReads > 100 {
			return 0, io.ErrNoProgress
		}
	} else {
		r.emptyReads = 0
	}
	return n, err
}

// boundedLayerReader fails closed when the compressed input exceeds its
// quota, rather than silently reporting EOF from an io.LimitReader.
// Read() explicitly probes for an extra source byte at the boundary.
type boundedLayerReader struct {
	ctx    context.Context
	reader io.Reader
	max    int64
	size   int64
	idle   int
}

func (r *boundedLayerReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.size >= r.max {
		var sentinel [1]byte
		n, err := r.reader.Read(sentinel[:])
		if n > 0 {
			return 0, errors.New("compressed OCI layer input quota exceeded")
		}
		if err != nil {
			return 0, err
		}
		r.idle++
		if r.idle > 100 {
			return 0, io.ErrNoProgress
		}
		return 0, nil
	}
	if int64(len(p)) > r.max-r.size {
		p = p[:int(r.max-r.size)]
	}
	n, err := r.reader.Read(p)
	if n < 0 || n > len(p) {
		return 0, errors.New("invalid OCI layer reader count")
	}
	r.size += int64(n)
	if n == 0 && err == nil {
		r.idle++
		if r.idle > 100 {
			return 0, io.ErrNoProgress
		}
	} else {
		r.idle = 0
	}
	return n, err
}
