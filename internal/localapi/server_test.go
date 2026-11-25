package localapi

import (
	"context"
	"github.com/GoreeCloud/containers/internal/engine"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrivateSocket(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot only")
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := engine.PrepareStateRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(base, "api.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, sock, state) }()
	for i := 0; i < 100; i++ {
		if _, e := os.Stat(sock); e == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	defer transport.CloseIdleConnections()
	for _, tc := range []struct {
		method, path string
		code         int
	}{{"GET", "/v1/health", 200}, {"GET", "/v1/journal", 200}, {"POST", "/v1/health", 400}, {"GET", "/v1/unknown", 404}, {"GET", "/v1/journal?x=1", 400}} {
		req, _ := http.NewRequest(tc.method, "http://local"+tc.path, nil)
		r, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != tc.code {
			t.Fatalf("%s: %d", tc.path, r.StatusCode)
		}
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("server did not stop")
	}
	if _, e := os.Lstat(sock); !os.IsNotExist(e) {
		t.Fatal("socket cleanup failed")
	}
}
