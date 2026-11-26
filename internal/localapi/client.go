package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Query contacts only a private local socket owned by the invoking UID.
// Both socket metadata and connected peer credentials must match. No TCP.
func Query(ctx context.Context, socket, endpoint string) (json.RawMessage, int, error) {
	if ctx == nil || os.Geteuid() == 0 || !filepath.IsAbs(socket) || filepath.Clean(socket) != socket {
		return nil, 0, errors.New("private absolute local socket required")
	}
	if endpoint != "health" && endpoint != "journal" && endpoint != "runtime-summary" {
		return nil, 0, errors.New("unsupported API endpoint")
	}
	parent := filepath.Dir(socket)
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return nil, 0, errors.New("unsafe socket parent")
	}
	p, e := os.Lstat(parent)
	if e != nil || !p.IsDir() || p.Mode().Perm()&0077 != 0 {
		return nil, 0, errors.New("private socket parent required")
	}
	st, ok := p.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return nil, 0, errors.New("socket parent owner mismatch")
	}
	before, e := os.Lstat(socket)
	if e != nil || before.Mode()&os.ModeSocket == 0 || before.Mode().Perm() != 0600 {
		return nil, 0, errors.New("unsafe socket file")
	}
	st, ok = before.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return nil, 0, errors.New("socket owner mismatch")
	}
	tr := &http.Transport{DisableKeepAlives: true, MaxResponseHeaderBytes: 8192, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		c, e := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socket)
		if e != nil {
			return nil, e
		}
		fail := func() (net.Conn, error) { c.Close(); return nil, errors.New("local peer verification failed") }
		u, ok := c.(*net.UnixConn)
		if !ok {
			return fail()
		}
		raw, e := u.SyscallConn()
		if e != nil {
			return fail()
		}
		var cred *syscall.Ucred
		var peerErr error
		e = raw.Control(func(fd uintptr) {
			cred, peerErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		})
		after, statErr := os.Lstat(socket)
		if e != nil || peerErr != nil || cred == nil || cred.Uid != uint32(os.Geteuid()) || statErr != nil || !os.SameFile(before, after) {
			return fail()
		}
		return c, nil
	}}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", "http://local/v1/"+endpoint, nil)
	if e != nil {
		return nil, 0, e
	}
	resp, e := c.Do(req)
	if e != nil {
		return nil, 0, errors.New("local API unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && !((endpoint == "journal" || endpoint == "runtime-summary") && resp.StatusCode == 409) {
		return nil, resp.StatusCode, errors.New("local API refused request")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if e != nil || len(b) > 16384 || !json.Valid(b) {
		return nil, 0, errors.New("invalid bounded local response")
	}
	validated, e := validateResponse(b, endpoint, resp.StatusCode)
	if e != nil {
		return nil, 0, e
	}
	return validated, resp.StatusCode, nil
}
