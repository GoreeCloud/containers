// Package localapi exposes a bounded, same-UID, read-only management preview.
package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/engine"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type peerListener struct {
	*net.UnixListener
	uid   uint32
	slots chan struct{}
}

func (l peerListener) Accept() (net.Conn, error) {
	for {
		c, e := l.AcceptUnix()
		if e != nil {
			return nil, e
		}
		raw, e := c.SyscallConn()
		if e != nil {
			c.Close()
			continue
		}
		var cred *syscall.Ucred
		var peerErr error
		e = raw.Control(func(fd uintptr) {
			cred, peerErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		})
		if e == nil && peerErr == nil && cred != nil && cred.Uid == l.uid {
			select {
			case l.slots <- struct{}{}:
				return &limitedConn{Conn: c, release: func() { <-l.slots }}, nil
			default:
				c.Close()
				continue
			}
		}
		c.Close()
	}
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }

// Handler exposes no mutations, identifiers, host paths or secrets.
func Handler(stateRoot string) http.Handler { return handler(stateRoot, nil) }
func handler(stateRoot string, runtime *engine.Engine) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" || r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			http.Error(w, "unsupported request", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/v1/health":
			json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "readOnly": true, "identityIntegrationVerified": false, "deploymentAuthorized": false})
		case "/v1/runtime-summary":
			if runtime == nil {
				http.NotFound(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			report, err := runtime.Inventory(ctx)
			if err != nil {
				http.Error(w, "runtime inventory unavailable", http.StatusServiceUnavailable)
				return
			}
			summary := runtimeSummary{ReadOnly: true, Total: len(report.Entries), Running: report.Running, Created: report.Created, Stopped: report.Stopped, Uncertain: report.Uncertain, RecoveryRequired: report.RecoveryRequired}
			if summary.RecoveryRequired {
				w.WriteHeader(http.StatusConflict)
			}
			json.NewEncoder(w).Encode(summary)
		case "/v1/journal":
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			report, err := engine.InspectJournal(ctx, stateRoot)
			if err != nil {
				http.Error(w, "journal unavailable", http.StatusServiceUnavailable)
				return
			}
			if report.RecoveryRequired {
				w.WriteHeader(http.StatusConflict)
			}
			json.NewEncoder(w).Encode(report)
		default:
			http.NotFound(w, r)
		}
	})
}

// Serve creates a fresh Unix socket in a caller-owned private directory.
// Foreground only; never exposes TCP or replaces an existing socket.
func Serve(ctx context.Context, socket, stateRoot string) error {
	return serve(ctx, socket, stateRoot, nil)
}

// ServeRuntime opts into a bounded redacted runtime census. Same-UID access
// is not MIPS identity. A runtime not bound to the supplied root is refused.
func ServeRuntime(ctx context.Context, socket, stateRoot string, runtime *engine.Engine) error {
	if runtime == nil || runtime.StateRoot != stateRoot {
		return errors.New("runtime root mismatch")
	}
	return serve(ctx, socket, stateRoot, runtime)
}
func serve(ctx context.Context, socket, stateRoot string, runtime *engine.Engine) error {
	if ctx == nil || os.Geteuid() == 0 || !filepath.IsAbs(socket) || filepath.Clean(socket) != socket {
		return errors.New("nonroot absolute socket required")
	}
	parent := filepath.Dir(socket)
	real, err := filepath.EvalSymlinks(parent)
	if err != nil || real != parent {
		return errors.New("unsafe socket parent")
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm()&0077 != 0 || st.Uid != uint32(os.Geteuid()) {
		return errors.New("private caller-owned socket parent required")
	}
	if _, err = os.Lstat(socket); !os.IsNotExist(err) {
		return errors.New("socket path already exists or is inaccessible")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return err
	}
	listener.SetUnlinkOnClose(false)
	defer listener.Close()
	original, err := os.Lstat(socket)
	if err != nil {
		return err
	}
	defer func() {
		if observed, e := os.Lstat(socket); e == nil && os.SameFile(original, observed) {
			_ = os.Remove(socket)
		}
	}()
	if err = os.Chmod(socket, 0600); err != nil {
		return err
	}
	server := &http.Server{Handler: handler(stateRoot, runtime), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 2 * time.Second, MaxHeaderBytes: 4096}
	server.SetKeepAlivesEnabled(false)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(stop)
		case <-done:
		}
	}()
	err = server.Serve(peerListener{listener, uint32(os.Geteuid()), make(chan struct{}, 16)})
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
