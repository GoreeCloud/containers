package ociimage

import (
	"bytes"
	"compress/gzip"
	"context"
	"testing"
)

func TestLayerRequiresCompleteTarEndMarker(t *testing.T) {
	valid := makeLayer(t, layerEntry{name: "fixture", size: 3, data: []byte("abc")})
	for _, compressed := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			data  []byte
			valid bool
		}{
			{"complete", valid, true},
			{"no-end-blocks", valid[:len(valid)-1024], false},
			{"single-end-block", valid[:len(valid)-512], false},
			{"partial-end-block", valid[:len(valid)-1], false},
			{"empty-stream", nil, false},
			{"empty-archive", make([]byte, 1024), true},
		} {
			t.Run(tc.name+map[bool]string{true: "-gzip", false: "-tar"}[compressed], func(t *testing.T) {
				data, media := tc.data, MediaLayer
				if compressed {
					var buffer bytes.Buffer
					w := gzip.NewWriter(&buffer)
					if _, err := w.Write(data); err != nil {
						t.Fatal(err)
					}
					if err := w.Close(); err != nil {
						t.Fatal(err)
					}
					data, media = buffer.Bytes(), MediaLayerGzip
				}
				_, err := InspectLayer(context.Background(), bytes.NewReader(data), media)
				if (err == nil) != tc.valid {
					t.Fatalf("valid=%t: %v", tc.valid, err)
				}
			})
		}
	}
}
