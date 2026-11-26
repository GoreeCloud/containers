package ociimage

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

type ManifestDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}
type ManifestDiscovery struct {
	Digest                   string               `json:"digest"`
	Size                     int64                `json:"size"`
	Config                   ManifestDescriptor   `json:"config"`
	Layers                   []ManifestDescriptor `json:"layers"`
	DigestMatches            bool                 `json:"digestMatches"`
	SourceProvenanceVerified bool                 `json:"sourceProvenanceVerified"`
	ExecutionAuthorized      bool                 `json:"executionAuthorized"`
}

func DiscoverManifest(ctx context.Context, origin, repo, digest string, size int64) (ManifestDiscovery, error) {
	tr := &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10}
	defer tr.CloseIdleConnections()
	return discoverManifest(ctx, &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, origin, repo, digest, size)
}
func discoverManifest(ctx context.Context, c *http.Client, origin, repo, digest string, size int64) (ManifestDiscovery, error) {
	return discoverManifestWithBytes(ctx, c, origin, repo, digest, size, nil)
}
func discoverManifestWithBytes(ctx context.Context, c *http.Client, origin, repo, digest string, size int64, admit func([]byte, ManifestDiscovery) error) (ManifestDiscovery, error) {

	var empty ManifestDiscovery
	if ctx == nil || size <= 0 || size > maxMetadataBytes {
		return empty, errors.New("invalid pinned manifest size")
	}
	endpoint, e := registryBlobURL(origin, repo, digest)
	if e != nil {
		return empty, e
	}
	endpoint = strings.Replace(endpoint, "/blobs/"+digest, "/manifests/"+digest, 1)
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if e != nil {
		return empty, errors.New("invalid manifest request")
	}
	req.Header.Set("Accept", MediaManifest)
	req.Header.Set("Accept-Encoding", "identity")
	resp, e := c.Do(req)
	if e != nil {
		return empty, errors.New("manifest HTTPS request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return empty, errors.New("manifest retrieval refused; auth and redirects unsupported")
	}
	media, _, e := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if e != nil || media != MediaManifest {
		return empty, errors.New("unsupported manifest response media type")
	}
	if resp.ContentLength >= 0 && resp.ContentLength != size {
		return empty, errors.New("manifest size mismatch")
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && !strings.EqualFold(enc, "identity") {
		return empty, errors.New("unexpected content encoding")
	}
	if pin := resp.Header.Get("Docker-Content-Digest"); pin != "" && pin != digest {
		return empty, errors.New("manifest header digest mismatch")
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, size+1))
	if e != nil || int64(len(raw)) != size || fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) != digest {
		return empty, errors.New("manifest descriptor mismatch")
	}
	if e = ctx.Err(); e != nil {
		return empty, e
	}
	result, e := parsePinnedManifest(raw, digest, size)
	if e != nil {
		return empty, e
	}
	if admit != nil {
		if e = admit(raw, result); e != nil {
			return empty, e
		}
	}
	return result, nil
}
func parsePinnedManifest(raw []byte, digest string, size int64) (ManifestDiscovery, error) {
	var empty ManifestDiscovery
	if size <= 0 || size > maxMetadataBytes || int64(len(raw)) != size || fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) != digest {
		return empty, errors.New("manifest descriptor mismatch")
	}
	obj, e := decodeObject(raw)
	if e != nil {
		return empty, e
	}
	if e = schemaTwo(obj); e != nil {
		return empty, e
	}
	if media, e := stringField(obj, "mediaType"); e != nil || media != MediaManifest {
		return empty, errors.New("unsupported manifest document")
	}
	if _, ok := obj["subject"]; ok {
		return empty, errors.New("artifact subject unsupported")
	}
	if _, ok := obj["artifactType"]; ok {
		return empty, errors.New("artifact type unsupported")
	}
	var config map[string]json.RawMessage
	if e = json.Unmarshal(obj["config"], &config); e != nil || config == nil {
		return empty, errors.New("manifest config missing")
	}
	cd, e := parseDescriptor(config)
	if e != nil || cd.Media != MediaConfig || cd.Size <= 0 || cd.Size > maxMetadataBytes {
		return empty, errors.New("invalid config descriptor")
	}
	layers, e := descriptors(obj["layers"])
	if e != nil || len(layers) > 8 {
		return empty, errors.New("manifest exceeds supported layer count")
	}
	report := ManifestDiscovery{Digest: digest, Size: size, Config: ManifestDescriptor{cd.Media, cd.Digest, cd.Size}, Layers: []ManifestDescriptor{}, DigestMatches: true}
	seen := map[string]descriptor{cd.Digest: cd}
	total := cd.Size
	for _, obj := range layers {
		ld, e := parseDescriptor(obj)
		if e != nil || ld.Size <= 0 || ld.Size > MaxLayerCompressedBytes || (ld.Media != MediaLayer && ld.Media != MediaLayerGzip) {
			return empty, errors.New("unsupported layer descriptor")
		}
		if prev, ok := seen[ld.Digest]; ok && prev != ld {
			return empty, errors.New("conflicting descriptor digest")
		}
		seen[ld.Digest] = ld
		if ld.Size > maxTotalBytes-total {
			return empty, errors.New("manifest aggregate descriptor size exceeded")
		}
		total += ld.Size
		report.Layers = append(report.Layers, ManifestDescriptor{ld.Media, ld.Digest, ld.Size})
	}
	// Ensure raw metadata never escapes the typed descriptor-only report.
	return report, nil
}
