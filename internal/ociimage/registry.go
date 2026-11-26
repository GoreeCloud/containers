package ociimage

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var registryRepository = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)
var registryHost = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// FetchRegistryBlob explicitly contacts one caller-selected HTTPS origin.
// No bearer token, credentials, redirect, proxy, tag resolution or upload.
// The digest is a caller pin, not a signature or provenance attestation.
func (s *Store) FetchRegistryBlob(ctx context.Context, origin, repository, digest string, size int64) (BlobInfo, error) {
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10,
	}
	defer transport.CloseIdleConnections()
	return s.fetchRegistryBlob(ctx, &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, origin, repository, digest, size)
}
func registryBlobURL(origin, repo, digest string) (string, error) {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || !registryHost.MatchString(u.Hostname()) || len(u.Host) > 253 || len(repo) > 255 || !registryRepository.MatchString(repo) || !digestPattern.MatchString(digest) {
		return "", errors.New("invalid HTTPS registry origin, repository or digest")
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid registry port")
		}
	}
	u.Path = "/v2/" + repo + "/blobs/" + digest
	return u.String(), nil
}
func (s *Store) fetchRegistryBlob(ctx context.Context, client *http.Client, origin, repo, digest string, size int64) (BlobInfo, error) {
	if ctx == nil || size <= 0 || size > s.maxBytes {
		return BlobInfo{}, errors.New("invalid registry blob descriptor")
	}
	endpoint, e := registryBlobURL(origin, repo, digest)
	if e != nil {
		return BlobInfo{}, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if e != nil {
		return BlobInfo{}, errors.New("invalid registry request")
	}
	req.Header.Set("Accept-Encoding", "identity")
	response, e := client.Do(req)
	if e != nil {
		return BlobInfo{}, errors.New("registry HTTPS request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return BlobInfo{}, errors.New("registry refused blob; authentication and redirects unsupported")
	}
	if response.ContentLength >= 0 && response.ContentLength != size {
		return BlobInfo{}, errors.New("registry content length mismatch")
	}
	if enc := response.Header.Get("Content-Encoding"); enc != "" && !strings.EqualFold(enc, "identity") {
		return BlobInfo{}, errors.New("unexpected registry content encoding")
	}
	if returned := response.Header.Get("Docker-Content-Digest"); returned != "" && returned != digest {
		return BlobInfo{}, errors.New("registry digest header mismatch")
	}
	return s.PutVerified(ctx, response.Body, digest, size)
}
