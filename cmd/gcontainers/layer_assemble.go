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

func imageAssemble(args []string, out io.Writer) error {
	if len(args) < 6 || len(args) > 34 || args[len(args)-1] != "--experimental" || (len(args)-2)%4 != 0 {
		return errors.New("usage: image assemble PRIVATE_PARENT (ABS_FILE tar|tar+gzip SHA256 SIZE){1,8} --experimental")
	}
	var layers []ociimage.LayerInput
	for i := 1; i < len(args)-1; i += 4 {
		p := args[i]
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return errors.New("clean absolute layer filename required")
		}
		parent := filepath.Dir(p)
		real, e := filepath.EvalSymlinks(parent)
		if e != nil || real != parent {
			return errors.New("symlinked layer parent")
		}
		var media string
		switch args[i+1] {
		case "tar":
			media = ociimage.MediaLayer
		case "tar+gzip":
			media = ociimage.MediaLayerGzip
		default:
			return errors.New("unsupported layer media")
		}
		size, e := strconv.ParseInt(args[i+3], 10, 64)
		if e != nil || size <= 0 || size > ociimage.MaxLayerCompressedBytes || strconv.FormatInt(size, 10) != args[i+3] {
			return errors.New("invalid descriptor size")
		}
		root, e := os.OpenRoot(parent)
		if e != nil {
			return e
		}
		defer root.Close()
		name := filepath.Base(p)
		info, e := root.Lstat(name)
		if e != nil || !info.Mode().IsRegular() || info.Size() != size {
			return errors.New("layer size or type rejected")
		}
		f, e := regularinput.Open(root, name, info)
		if e != nil {
			return e
		}
		defer f.Close()
		layers = append(layers, ociimage.LayerInput{Reader: f, Media: media, Digest: args[i+2], Size: size})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	report, e := ociimage.StageLayers(ctx, layers, args[0])
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(report)
}
