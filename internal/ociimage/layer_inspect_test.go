package ociimage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"testing"
)

type layerEntry struct {
	name string
	kind byte
	size int64
	link string
	data []byte
}

func makeLayer(t *testing.T, entries ...layerEntry) []byte {
	t.Helper()
	var out bytes.Buffer
	w := tar.NewWriter(&out)
	for _, entry := range entries {
		typ := entry.kind
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: entry.name, Typeflag: typ, Mode: 0o644, Size: entry.size, Linkname: entry.link}
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg && entry.size > 0 {
			if _, err := w.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestInspectSyntheticTarAndGzip(t *testing.T) {
	original := makeLayer(t,
		layerEntry{name: "etc/", kind: tar.TypeDir},
		layerEntry{name: "etc/config.txt", size: 4, data: []byte("data")},
		layerEntry{name: "readme", size: 0},
	)
	for _, compressed := range []bool{false, true} {
		input := original
		media := MediaLayer
		if compressed {
			var out bytes.Buffer
			zw := gzip.NewWriter(&out)
			if _, err := zw.Write(original); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			input = out.Bytes()
			media = MediaLayerGzip
		}
		r, err := InspectLayer(context.Background(), bytes.NewReader(input), media)
		if err != nil {
			t.Fatal(err)
		}
		if r.FileCount != 2 || r.DirectoryCount != 1 || r.UnpackedBytes != 4 {
			t.Fatalf("report %+v", r)
		}
	}
}
func TestInspectLayerRejectsDangerousEntries(t *testing.T) {
	cases := map[string][]byte{
		"escape":         makeLayer(t, layerEntry{name: "../../etc/passwd", size: 1, data: []byte("x")}),
		"absolute":       makeLayer(t, layerEntry{name: "/etc/passwd", size: 1, data: []byte("x")}),
		"symlink":        makeLayer(t, layerEntry{name: "redirect", kind: tar.TypeSymlink, link: "/etc/shadow"}),
		"hardlink":       makeLayer(t, layerEntry{name: "redirect", kind: tar.TypeLink, link: "etc/hosts"}),
		"device":         makeLayer(t, layerEntry{name: "dev/x", kind: tar.TypeChar}),
		"whiteout":       makeLayer(t, layerEntry{name: "etc/.wh.secret"}),
		"duplicate":      makeLayer(t, layerEntry{name: "same"}, layerEntry{name: "same"}),
		"path collision": makeLayer(t, layerEntry{name: "a/b"}, layerEntry{name: "a"}),
		"bad ancestor":   makeLayer(t, layerEntry{name: "a"}, layerEntry{name: "a/b"}),
		"oversize":       makeLayerHeaderOnly(t, "huge", MaxLayerUnpackedBytes+1),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := InspectLayer(context.Background(), bytes.NewReader(src), MediaLayer); err == nil {
				t.Fatal("unsafe OCI archive accepted")
			}
		})
	}
}
func makeLayerHeaderOnly(t *testing.T, name string, size int64) []byte {
	t.Helper()
	var b bytes.Buffer
	// Build a valid header without allocating a massive payload.
	w := tar.NewWriter(&b)
	if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: size, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestInspectLayerRejectsBadTrailerAndUnsupportedMedia(t *testing.T) {
	valid := makeLayer(t, layerEntry{name: "ok", size: 1, data: []byte("x")})
	if _, err := InspectLayer(context.Background(), bytes.NewReader(valid), "application/vnd.oci.image.layer.v1.tar+zstd"); err == nil {
		t.Fatal("unsupported zstd media accepted")
	}
	invalid := append(append([]byte(nil), valid...), []byte("garbage")...)
	if _, err := InspectLayer(context.Background(), bytes.NewReader(invalid), MediaLayer); err == nil {
		t.Fatal("nonzero tar suffix accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectLayer(ctx, bytes.NewReader(valid), MediaLayer); err == nil {
		t.Fatal("cancelled inspection accepted")
	}
}
func TestInspectLayerRejectsTruncation(t *testing.T) {
	original := makeLayer(t, layerEntry{name: "file", size: 10, data: []byte(strings.Repeat("x", 10))})
	if _, err := InspectLayer(context.Background(), bytes.NewReader(original[:500]), MediaLayer); err == nil {
		t.Fatal("truncated layer accepted")
	}
}
func TestLayerContextReaderRejectsStall(t *testing.T) {
	r := &layerContextReader{ctx: context.Background(), reader: zeroReader{}}
	buf := make([]byte, 5)
	var err error
	for i := 0; i < 101; i++ {
		_, err = r.Read(buf)
	}
	if err != io.ErrNoProgress {
		t.Fatalf("zero-progress source accepted: %v", err)
	}
}
func FuzzLayerTarInspection(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("not a tar"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		_, _ = InspectLayer(context.Background(), bytes.NewReader(data), MediaLayer)
	})
}
