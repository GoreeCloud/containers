package ociimage

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

type IndexSelection struct {
	IndexDigest              string             `json:"indexDigest"`
	Architecture             string             `json:"architecture"`
	OS                       string             `json:"os"`
	Manifest                 ManifestDescriptor `json:"manifest"`
	Config                   ManifestDescriptor `json:"config"`
	LayerCount               int                `json:"layerCount"`
	MetadataVerified         bool               `json:"metadataVerified"`
	LayerContentsVerified    bool               `json:"layerContentsVerified"`
	SourceProvenanceVerified bool               `json:"sourceProvenanceVerified"`
	ExecutionAuthorized      bool               `json:"executionAuthorized"`
}

var platformWord = regexp.MustCompile("^[a-z0-9][a-z0-9_.-]{0,31}$")

// SelectIndex verifies an explicit local OCI index pin, selects exactly one
// unambiguous supported platform and verifies its pinned manifest and config.
// It does not fetch missing bytes, read layer contents or attest provenance.
func (s *Store) SelectIndex(ctx context.Context, digest string, size int64, arch string) (IndexSelection, error) {
	var empty IndexSelection
	if ctx == nil || s == nil || (arch != "amd64" && arch != "arm64") {
		return empty, errors.New("explicit linux amd64 or arm64 required")
	}
	root, e := s.openRoot()
	if e != nil {
		return empty, e
	}
	defer root.Close()
	raw, e := readPinnedMetadata(ctx, root, ManifestDescriptor{MediaIndex, digest, size})
	if e != nil {
		return empty, e
	}
	obj, e := decodeObject(raw)
	if e != nil {
		return empty, e
	}
	if e = schemaTwo(obj); e != nil {
		return empty, e
	}
	media, e := stringField(obj, "mediaType")
	if e != nil || media != MediaIndex {
		return empty, errors.New("explicit OCI index media required")
	}
	for _, k := range []string{"artifactType", "subject"} {
		if _, ok := obj[k]; ok {
			return empty, errors.New("artifact index unsupported")
		}
	}
	children, e := descriptors(obj["manifests"])
	if e != nil || len(children) == 0 || len(children) > 32 {
		return empty, errors.New("one to 32 index manifests required")
	}
	var selected ManifestDescriptor
	seen := map[string]bool{}
	for _, child := range children {
		d, e := parseDescriptor(child)
		if e != nil || d.Media != MediaManifest || d.Size <= 0 || d.Size > maxMetadataBytes {
			return empty, errors.New("only direct OCI manifest descriptors supported")
		}
		if seen[d.Digest] {
			return empty, errors.New("duplicate index manifest digest")
		}
		seen[d.Digest] = true
		if _, ok := child["artifactType"]; ok {
			return empty, errors.New("artifact descriptor unsupported")
		}
		p, e := decodeObject(child["platform"])
		if e != nil {
			return empty, errors.New("explicit index platform required")
		}
		for k := range p {
			if k != "os" && k != "architecture" && k != "variant" {
				return empty, errors.New("unsupported platform constraint")
			}
		}
		osName, e := stringField(p, "os")
		if e != nil || !platformWord.MatchString(osName) {
			return empty, errors.New("invalid platform OS")
		}
		a, e := stringField(p, "architecture")
		if e != nil || !platformWord.MatchString(a) {
			return empty, errors.New("invalid platform architecture")
		}
		variant := ""
		if _, ok := p["variant"]; ok {
			if e = json.Unmarshal(p["variant"], &variant); e != nil || !platformWord.MatchString(variant) {
				return empty, errors.New("invalid platform variant")
			}
		}
		if osName != "linux" || a != arch {
			continue
		}
		// Explicit narrow baseline; optimized variants require separate selection.
		if variant != "" && !(arch == "arm64" && variant == "v8") {
			continue
		}
		if selected.Digest != "" {
			return empty, errors.New("ambiguous index platform")
		}
		selected = ManifestDescriptor{d.Media, d.Digest, d.Size}
	}
	if selected.Digest == "" {
		return empty, errors.New("no supported index platform")
	}
	m, e := readPinnedMetadata(ctx, root, selected)
	if e != nil {
		return empty, e
	}
	manifest, e := parsePinnedManifest(m, selected.Digest, selected.Size)
	if e != nil {
		return empty, e
	}
	if len(manifest.Layers) == 0 {
		return empty, errors.New("empty manifest unsupported")
	}
	c, e := readPinnedMetadata(ctx, root, manifest.Config)
	if e != nil {
		return empty, e
	}
	configArch, _, e := configDiffIDs(c, len(manifest.Layers))
	if e != nil || configArch != arch {
		return empty, errors.New("index and config platform mismatch")
	}
	config, e := decodeObject(c)
	if e != nil {
		return empty, e
	}
	if _, ok := config["variant"]; ok {
		v, e := stringField(config, "variant")
		if e != nil || !(arch == "arm64" && v == "v8") {
			return empty, errors.New("unsupported config variant")
		}
	}
	if e = ctx.Err(); e != nil {
		return empty, e
	}
	return IndexSelection{IndexDigest: digest, Architecture: arch, OS: "linux", Manifest: selected, Config: manifest.Config, LayerCount: len(manifest.Layers), MetadataVerified: true}, nil
}
