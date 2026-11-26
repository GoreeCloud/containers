package ociimage

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"time"
)

type PullStageReport struct {
	StoreAssemblyReport
	DownloadedBlobs int `json:"downloadedBlobs"`
	ReusedBlobs     int `json:"reusedBlobs"`
}

// PullAndStage is explicitly selected by a local operator. It performs only
// digest-pinned HTTPS GET requests to one origin and inert staging. Verified
// CAS objects may remain after a later error; no image tag or executable
// image is published. It never sends credentials, resolves tags, or executes.
func (s *Store) PullAndStage(ctx context.Context, origin, repo, digest string, size int64, parent string) (PullStageReport, error) {
	tr := &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s.pullAndStage(ctx, c, origin, repo, digest, size, parent)
}
func (s *Store) pullAndStage(ctx context.Context, c *http.Client, origin, repo, digest string, size int64, parent string) (PullStageReport, error) {
	var empty PullStageReport
	if s == nil || ctx == nil {
		return empty, errors.New("store and context required")
	}
	if _, e := registryBlobURL(origin, repo, digest); e != nil {
		return empty, e
	}
	if size <= 0 || size > maxMetadataBytes {
		return empty, errors.New("invalid manifest descriptor size")
	}
	// Refuse unsafe prior store content before any request. No automatic repair.
	audit, e := s.Audit(ctx)
	if e != nil || !audit.Verified || audit.OperatorReviewRequired {
		return empty, errors.New("existing store requires review")
	}
	report := PullStageReport{}
	root, e := s.openRoot()
	if e != nil {
		return empty, e
	}
	defer root.Close()
	check := func(d ManifestDescriptor) (bool, error) {
		name := "blobs/sha256/" + d.Digest[7:]
		_, e := root.Lstat(name)
		if os.IsNotExist(e) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		f, e := openPinnedStoreBlob(root, d)
		if e != nil {
			return false, e
		}
		f.Close()
		n, e := s.Verify(ctx, d.Digest)
		if e != nil || n != d.Size {
			return false, errors.New("existing pinned store blob mismatch")
		}
		return true, nil
	}
	manifestDesc := ManifestDescriptor{MediaManifest, digest, size}
	found, e := check(manifestDesc)
	if e != nil {
		return empty, e
	}
	var manifest ManifestDiscovery
	if found {
		raw, e := readPinnedMetadata(ctx, root, manifestDesc)
		if e != nil {
			return empty, e
		}
		manifest, e = parsePinnedManifest(raw, digest, size)
		if e != nil {
			return empty, e
		}
		report.ReusedBlobs++
	} else {
		manifest, e = discoverManifestWithBytes(ctx, c, origin, repo, digest, size, func(raw []byte, _ ManifestDiscovery) error {
			_, e := s.PutVerified(ctx, bytes.NewReader(raw), digest, size)
			return e
		})
		if e != nil {
			return empty, e
		}
		report.DownloadedBlobs++
	}
	if len(manifest.Layers) == 0 {
		return empty, errors.New("empty image assembly unsupported")
	}
	acquire := func(d ManifestDescriptor) error {
		found, e := check(d)
		if e != nil {
			return e
		}
		if found {
			report.ReusedBlobs++
			return nil
		}
		if _, e = s.fetchRegistryBlob(ctx, c, origin, repo, d.Digest, d.Size); e != nil {
			return e
		}
		report.DownloadedBlobs++
		return nil
	}
	// Check configuration before transferring potentially large layers.
	if e = acquire(manifest.Config); e != nil {
		return empty, e
	}
	raw, e := readPinnedMetadata(ctx, root, manifest.Config)
	if e != nil {
		return empty, e
	}
	if _, _, e = configDiffIDs(raw, len(manifest.Layers)); e != nil {
		return empty, e
	}
	seen := map[string]bool{digest: true, manifest.Config.Digest: true}
	for _, d := range manifest.Layers {
		if seen[d.Digest] {
			continue
		}
		if e = acquire(d); e != nil {
			return empty, e
		}
		seen[d.Digest] = true
	}
	staged, e := s.AssembleManifest(ctx, digest, size, parent)
	if e != nil {
		return empty, e
	}
	report.StoreAssemblyReport = staged
	return report, nil
}
