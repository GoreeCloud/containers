package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/ociimage"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func imageStageLayer(args []string, out io.Writer) error {
	if len(args) != 6 || args[5] != "--experimental" {
		return errors.New("usage: image stage-layer ABS_FILE tar|tar+gzip sha256:HEX DESCRIPTOR_BYTES PRIVATE_PARENT --experimental")
	}
	source := args[0]
	if !filepath.IsAbs(source) || filepath.Clean(source) != source {
		return errors.New("clean absolute layer source required")
	}
	parent := filepath.Dir(source)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return errors.New("symlinked layer parent unsupported")
	}
	var media string
	switch args[1] {
	case "tar":
		media = ociimage.MediaLayer
	case "tar+gzip":
		media = ociimage.MediaLayerGzip
	default:
		return errors.New("unsupported layer media")
	}
	size, err := strconv.ParseInt(args[3], 10, 64)
	if err != nil || size <= 0 || size > ociimage.MaxLayerCompressedBytes || strconv.FormatInt(size, 10) != args[3] {
		return errors.New("canonical positive descriptor size required")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(source)
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return errors.New("layer size or source type rejected")
	}
	file, err := regularinput.Open(root, name, info)
	if err != nil {
		return err
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := ociimage.StageLayer(ctx, file, media, args[2], size, args[4])
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(report)
}
