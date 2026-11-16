// Package ociimage implements a read-only, bounded OCI image-layout verifier.
// It never unpacks, executes, downloads, or imports image contents.
package ociimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const (
	maxMetadataBytes int64 = 4 << 20
	maxBlobBytes     int64 = 512 << 20
	maxTotalBytes    int64 = 1 << 30
	maxDescriptors         = 128
	maxDepth               = 4
)
const (
	MediaIndex     = "application/vnd.oci.image.index.v1+json"
	MediaManifest  = "application/vnd.oci.image.manifest.v1+json"
	MediaConfig    = "application/vnd.oci.image.config.v1+json"
	MediaLayer     = "application/vnd.oci.image.layer.v1.tar"
	MediaLayerGzip = "application/vnd.oci.image.layer.v1.tar+gzip"
	MediaLayerZstd = "application/vnd.oci.image.layer.v1.tar+zstd"
)

var digestPattern = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

type Report struct {
	ManifestCount int
	VerifiedBlobs int
	VerifiedBytes int64
}
type descriptor struct {
	Media  string
	Digest string
	Size   int64
}
type verifier struct {
	ctx     context.Context
	root    *os.Root
	report  Report
	visited map[string]descriptor
	busy    map[string]bool
}

func VerifyLayout(ctx context.Context, base string) (Report, error) {
	var empty Report
	if ctx == nil {
		return empty, errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	rootPath, err := filepath.Abs(base)
	if err != nil {
		return empty, err
	}
	if err := regularDir(rootPath); err != nil {
		return empty, fmt.Errorf("image root: %w", err)
	}
	before, err := os.Lstat(rootPath)
	if err != nil {
		return empty, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	opened, err := root.Lstat(".")
	if err != nil || !os.SameFile(before, opened) {
		return empty, errors.New("OCI layout root changed while opening")
	}
	for _, name := range []string{"blobs", "blobs/sha256"} {
		fi, err := root.Lstat(name)
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return empty, errors.New("OCI layout blob directories must not be symlinks")
		}
	}
	layout, err := readObject(root, "oci-layout", 4096)
	if err != nil {
		return empty, fmt.Errorf("OCI layout marker: %w", err)
	}
	version, err := stringField(layout, "imageLayoutVersion")
	if err != nil || version != "1.0.0" {
		return empty, errors.New("unsupported OCI layout version")
	}
	index, err := readObject(root, "index.json", maxMetadataBytes)
	if err != nil {
		return empty, fmt.Errorf("OCI index: %w", err)
	}
	v := &verifier{ctx: ctx, root: root, visited: map[string]descriptor{}, busy: map[string]bool{}}
	if err := v.index(index, 0); err != nil {
		return empty, err
	}
	return v.report, nil
}

func regularDir(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("directory or symlink not allowed")
	}
	return nil
}
func readObject(root *os.Root, relative string, max int64) (map[string]json.RawMessage, error) {
	fi, err := root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > max {
		return nil, errors.New("nonregular or oversize OCI metadata")
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(fi, opened) || !opened.Mode().IsRegular() {
		return nil, errors.New("OCI metadata file replaced while opening")
	}
	blob, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(blob)) > max || int64(len(blob)) != fi.Size() {
		return nil, errors.New("OCI metadata size changed during read")
	}
	return decodeObject(blob)
}
func decodeObject(blob []byte) (map[string]json.RawMessage, error) {
	if err := uniqueJSON(blob); err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(blob, &obj); err != nil || obj == nil {
		return nil, errors.New("invalid OCI JSON object")
	}
	return obj, nil
}

func uniqueJSON(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if err := walk(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func walk(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON too deeply nested")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || keys[key] {
				return errors.New("duplicate or invalid JSON key")
			}
			keys[key] = true
			if err := walk(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := walk(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}
func stringField(obj map[string]json.RawMessage, key string) (string, error) {
	raw, ok := obj[key]
	if !ok {
		return "", errors.New("missing string field")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errors.New("invalid string field")
	}
	return value, nil
}
func schemaTwo(obj map[string]json.RawMessage) error {
	var n int
	if err := json.Unmarshal(obj["schemaVersion"], &n); err != nil || n != 2 {
		return errors.New("unsupported OCI schemaVersion")
	}
	return nil
}
func descriptors(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	var values []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, errors.New("invalid descriptor array")
	}
	if len(values) > maxDescriptors {
		return nil, errors.New("too many descriptors")
	}
	return values, nil
}
func parseDescriptor(obj map[string]json.RawMessage) (descriptor, error) {
	var d descriptor
	var err error
	d.Media, err = stringField(obj, "mediaType")
	if err != nil {
		return d, err
	}
	d.Digest, err = stringField(obj, "digest")
	if err != nil {
		return d, err
	}
	if !digestPattern.MatchString(d.Digest) {
		return d, errors.New("invalid or unsupported OCI digest")
	}
	if err := json.Unmarshal(obj["size"], &d.Size); err != nil || d.Size < 0 || d.Size > maxBlobBytes {
		return d, errors.New("invalid or oversize OCI descriptor size")
	}
	if raw, ok := obj["urls"]; ok && string(raw) != "null" {
		return d, errors.New("external OCI blob URLs unsupported")
	}
	if _, ok := obj["data"]; ok {
		return d, errors.New("inline OCI descriptor data unsupported")
	}
	return d, nil
}
func (v *verifier) index(obj map[string]json.RawMessage, depth int) error {
	if depth > maxDepth {
		return errors.New("nested OCI index limit exceeded")
	}
	if err := schemaTwo(obj); err != nil {
		return err
	}
	if media, ok := obj["mediaType"]; ok && string(media) != `"`+MediaIndex+`"` {
		return errors.New("unsupported OCI index media type")
	}
	manifestList, ok := obj["manifests"]
	if !ok {
		return errors.New("OCI index requires manifests")
	}
	children, err := descriptors(manifestList)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return errors.New("OCI index contains no manifests in preview profile")
	}
	for _, item := range children {
		d, err := parseDescriptor(item)
		if err != nil {
			return err
		}
		if d.Media != MediaIndex && d.Media != MediaManifest {
			return errors.New("unsupported OCI index child type")
		}
		if err := v.visit(d, depth+1); err != nil {
			return err
		}
	}
	return nil
}
func (v *verifier) manifest(obj map[string]json.RawMessage, depth int) error {
	if err := schemaTwo(obj); err != nil {
		return err
	}
	if media, ok := obj["mediaType"]; ok && string(media) != `"`+MediaManifest+`"` {
		return errors.New("unsupported OCI manifest media type")
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(obj["config"], &config); err != nil || config == nil {
		return errors.New("OCI manifest missing config")
	}
	cd, err := parseDescriptor(config)
	if err != nil {
		return err
	}
	if cd.Media != MediaConfig {
		return errors.New("unsupported OCI config media type")
	}
	if err := v.visit(cd, depth+1); err != nil {
		return err
	}
	raw, ok := obj["layers"]
	if !ok {
		return errors.New("OCI manifest requires layers")
	}
	layers, err := descriptors(raw)
	if err != nil {
		return err
	}
	for _, layer := range layers {
		ld, err := parseDescriptor(layer)
		if err != nil {
			return err
		}
		switch ld.Media {
		case MediaLayer, MediaLayerGzip, MediaLayerZstd:
		default:
			return errors.New("unsupported OCI layer media type")
		}
		if err := v.visit(ld, depth+1); err != nil {
			return err
		}
	}
	v.report.ManifestCount++
	return nil
}
func (v *verifier) visit(d descriptor, depth int) error {
	if depth > maxDepth {
		return errors.New("OCI descriptor depth exceeded")
	}
	if err := v.ctx.Err(); err != nil {
		return err
	}
	key := d.Digest
	if v.busy[key] {
		return errors.New("recursive OCI image descriptor")
	}
	if previous, ok := v.visited[key]; ok {
		if previous != d {
			return errors.New("conflicting OCI descriptor metadata for one digest")
		}
		return nil
	}
	if v.report.VerifiedBlobs >= maxDescriptors || v.report.VerifiedBytes+d.Size > maxTotalBytes {
		return errors.New("OCI verification quota exceeded")
	}
	relative := "blobs/sha256/" + d.Digest[len("sha256:"):]
	fi, err := v.root.Lstat(relative)
	if err != nil {
		return fmt.Errorf("missing OCI blob: %w", err)
	}
	if !fi.Mode().IsRegular() || fi.Size() != d.Size {
		return errors.New("OCI blob is nonregular or descriptor size differs")
	}
	metadata := d.Media == MediaManifest || d.Media == MediaIndex
	if metadata && d.Size > maxMetadataBytes {
		return errors.New("OCI manifest/index metadata too large")
	}
	file, err := v.root.Open(relative)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(fi, opened) || !opened.Mode().IsRegular() {
		return errors.New("OCI blob changed between metadata check and open")
	}
	h := sha256.New()
	var snapshot bytes.Buffer
	buffer := make([]byte, 32<<10)
	var count int64
	for {
		if err := v.ctx.Err(); err != nil {
			return err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			count += int64(n)
			if count > d.Size {
				return errors.New("OCI blob grew during verification")
			}
			h.Write(buffer[:n])
			// Snapshot index/manifest bytes while hashing the SAME opened
			// descriptor, avoiding a second pathname read and TOCTOU gap.
			if metadata {
				snapshot.Write(buffer[:n])
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if count != d.Size || hex.EncodeToString(h.Sum(nil)) != d.Digest[len("sha256:"):] {
		return errors.New("OCI blob digest or size mismatch")
	}

	v.visited[key] = d
	v.report.VerifiedBlobs++
	v.report.VerifiedBytes += count
	if d.Media != MediaManifest && d.Media != MediaIndex {
		return nil
	}
	v.busy[key] = true
	defer delete(v.busy, key)
	child, err := decodeObject(snapshot.Bytes())
	if err != nil {
		return err
	}
	if d.Media == MediaIndex {
		return v.index(child, depth)
	}
	return v.manifest(child, depth)
}
