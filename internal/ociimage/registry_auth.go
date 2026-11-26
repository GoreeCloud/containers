package ociimage

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/GoreeCloud/containers/internal/regularinput"
)

const maxRegistryTokenBytes = 4096

var bearerGrammar = regexp.MustCompile("^[A-Za-z0-9._~+/-]+=*$")

// Only explicitly selected owner-private local token files are accepted.
// No discovery from environment, credential stores, redirects or challenges.
func readRegistryBearer(name string) (string, error) {
	if os.Geteuid() == 0 || !filepath.IsAbs(name) || filepath.Clean(name) != name {
		return "", errors.New("private absolute registry token file required")
	}
	parent := filepath.Dir(name)
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return "", errors.New("unsafe registry token parent")
	}
	before, e := os.Lstat(parent)
	if e != nil || privateStoreInfo(before) != nil {
		return "", errors.New("unsafe registry token parent")
	}
	root, e := os.OpenRoot(parent)
	if e != nil {
		return "", errors.New("registry token unavailable")
	}
	defer root.Close()
	actual, e := root.Stat(".")
	if e != nil || !os.SameFile(before, actual) {
		return "", errors.New("registry token parent changed")
	}
	n := filepath.Base(name)
	info, e := root.Lstat(n)
	if e != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxRegistryTokenBytes+1 || (info.Mode().Perm() != 0600 && info.Mode().Perm() != 0400) || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return "", errors.New("unsafe registry token file")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return "", errors.New("unsafe registry token ownership")
	}
	f, e := regularinput.Open(root, n, info)
	if e != nil {
		return "", errors.New("registry token unavailable")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxRegistryTokenBytes+2))
	if e != nil || int64(len(b)) != info.Size() {
		return "", errors.New("registry token changed")
	}
	if e = verifyLayoutReadIdentity(root, f, info, n); e != nil {
		return "", errors.New("registry token changed")
	}
	now, e := f.Stat()
	if e != nil {
		return "", errors.New("registry token changed")
	}
	st2, ok := now.Sys().(*syscall.Stat_t)
	if !ok || st.Ctim != st2.Ctim || now.Mode() != info.Mode() {
		return "", errors.New("registry token changed")
	}
	token := strings.TrimSuffix(string(b), "\n")
	if len(token) < 1 || len(token) > maxRegistryTokenBytes || !bearerGrammar.MatchString(token) {
		return "", errors.New("invalid registry token format")
	}
	return token, nil
}

type scopedRegistryTransport struct {
	next              http.RoundTripper
	host, repo, token string
}

func (t scopedRegistryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Restrict every credential-bearing request, even if an internal caller
	// later adds an unrelated registry URL or redirect policy.
	if r == nil || r.URL == nil || r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != t.host || r.URL.User != nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" || r.URL.RawPath != "" || r.URL.Opaque != "" || r.Host != "" && r.Host != t.host || r.Header.Get("Authorization") != "" || r.Body != nil {
		return nil, errors.New("registry credential scope refused")
	}
	prefix := "/v2/" + t.repo + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return nil, errors.New("registry credential repository scope refused")
	}
	suffix := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(suffix, "/")
	if len(parts) != 2 || (parts[0] != "manifests" && parts[0] != "blobs") || !digestPattern.MatchString(parts[1]) {
		return nil, errors.New("registry credential descriptor scope refused")
	}
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.next.RoundTrip(clone)
}
func scopedClient(transport http.RoundTripper, origin, repo, digest, token string) (*http.Client, error) {
	if _, e := registryBlobURL(origin, repo, digest); e != nil {
		return nil, e
	}
	if transport == nil || len(token) < 1 || len(token) > maxRegistryTokenBytes || !bearerGrammar.MatchString(token) {
		return nil, errors.New("invalid scoped registry credential")
	}
	u, e := url.Parse(origin)
	if e != nil {
		return nil, errors.New("invalid registry origin")
	}
	return &http.Client{Transport: scopedRegistryTransport{transport, u.Host, repo, token}, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

// PullAndStageAuthenticated accepts a pre-issued read-scoped bearer token
// for one explicit HTTPS registry/repository. It never logs, persists,
// discovers, refreshes, forwards, or exchanges credentials; cache hits
// do not establish that remote authentication took place.
func (s *Store) PullAndStageAuthenticated(ctx context.Context, origin, repo, digest string, size int64, parent, tokenFile string) (PullStageReport, error) {
	return s.PullAndStageAuthenticatedWithPolicy(ctx, origin, repo, digest, size, parent, tokenFile, false)
}
func (s *Store) PullAndStageAuthenticatedWithPolicy(ctx context.Context, origin, repo, digest string, size int64, parent, tokenFile string, whiteouts bool) (PullStageReport, error) {

	var empty PullStageReport
	if s == nil || ctx == nil {
		return empty, errors.New("store and context required")
	}
	if _, e := registryBlobURL(origin, repo, digest); e != nil {
		return empty, e
	}
	if e := ctx.Err(); e != nil {
		return empty, e
	}
	token, e := readRegistryBearer(tokenFile)
	if e != nil {
		return empty, e
	}
	tr := &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10}
	defer tr.CloseIdleConnections()
	client, e := scopedClient(tr, origin, repo, digest, token)
	if e != nil {
		return empty, e
	}
	return s.pullAndStagePolicy(ctx, client, origin, repo, digest, size, parent, whiteouts)
}
