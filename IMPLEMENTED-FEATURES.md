# GoreeCloud Containers — Implemented Features

> **Authority:** Repository-native implemented-feature record, seeded from the existing `FEATURES.md`. Existing Development/Partial/Planned boundaries remain controlling.

## GoreeCloud Containers — Features

## Current Development features

The following capabilities exist in the current source and are Development-only:

- Validated GoreeCloud container identifiers.
- Explicit container lifecycle states and guarded state transitions.
- Deterministic in-memory state for development and tests.
- Typed minimal Linux OCI configuration generation and controlled no-overwrite bundle initialization.
- Bundle-publication integrity guards reject unexpected OCI root paths and malformed mutable process fields before writing the owner-only configuration file.
- Experimental `goree bundle init-rootless` configuration generation with an explicit user namespace, single-ID UID/GID maps to non-root host identities, a read-only root filesystem, `noNewPrivileges`, and a restricted `/proc` mount validated against a real, non-symlink mountpoint. An opt-in disposable `runc` rootless smoke test passed on one Debian VPS; broad native runtime acceptance remains pending.
- `crun` and `runc` runtime identities, version probing, and low-level `create`, `start`, `state`, and `delete` execution.
- Two independent, explicitly opted-in no-wrapper Development lifecycle tests on a non-root Debian VPS using real `/usr/bin/runc`; a minimal test bundle successfully completed `create → state → start → state → delete`, verified mapped UID/GID and procfs, preserved invalid-rootfs diagnostics, and cleaned up isolated state. This is not broad runtime acceptance.
- Absolute/canonical runtime-path and bundle/config endpoint validation.
- Direct child-process invocation without a command shell; bounded, nonblocking Unix stdout/stderr capture that does not wait on descendant-held pipe EOF; preserved, capped OCI `create` error diagnostics; configured runtime timeout handling and explicit failure propagation.
- Strict lowercase SHA-256 digest parsing and verification.
- Bounded content-addressed image store with verified existing-content reuse.
- Development retrieval of supported OCI Image Manifest and Docker Registry v2 image manifests.
- Development retrieval and parsing of OCI/Docker image configuration and supported image layers.
- Bounded anonymous Bearer-token challenge handling for pull scopes; public/non-loopback transport requires HTTPS.
- Manifest/config/layer verification before acceptance into the verified content path.
- Linux image validation and image-config layer/diff-ID consistency checks.
- Supported uncompressed tar and gzip-compressed OCI/Docker layer handling.
- Uncompressed diff-ID verification before rootfs publication.
- Restricted layer extraction with path traversal, symlink-parent, per-entry, per-layer entry-count, and unpacked-size protections.
- OCI whiteout and opaque-whiteout handling for supported entries.
- Staged construction of a new rootfs target without merging into an existing target.
- Development CLI commands `goree image ingest` and `goree image pull`.
- Deterministic fixture-registry, content-store, rootfs, fake-runtime, timeout, and failure-path tests.
- Rust CI and GoreeCloud Platform Contract declaration.

## Current Development limitations / pending acceptance

The current source does not establish:

- Registry username/password or other reusable-credential authentication.
- OCI image-index/Docker manifest-list selection or multi-platform resolution.
- Symbolic-link or hard-link archive entry extraction.
- Image signature, attestation, SBOM, provenance, or trust-policy verification.
- Real external-registry interoperability acceptance.
- Broad real `crun`/`runc` lifecycle acceptance across workloads and environments or full OCI conformance; only an isolated minimal direct `runc` rootless lifecycle smoke has passed.
- Complete rootless isolation/resource-boundary acceptance beyond the verified disposable user-namespace identity/procfs lifecycle.
- A high-level image-to-container `run` workflow.
- Persistent engine metadata and recovery acceptance.
- Complete logs/run/stop/inspect/remove engine workflows.

## Later planned features

Networking, volumes, health checks, restart policies, Compose compatibility, image builds, remote management, Docker-compatible interoperability endpoints, native management clients, web administration, and accepted GoreeCloud platform-system integrations remain later work.

No pending or planned item in this document should be interpreted as current product availability.
