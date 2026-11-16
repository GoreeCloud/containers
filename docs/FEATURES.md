# GoreeCloud Containers — Feature status

| Capability | Status | Evidence / limitation |
| --- | --- | --- |
| First-party Go command-line prototype | Development | cmd/gcontainers; Linux CI |
| OCI bundle policy validation | Development | internal/bundle; strict process bounds and caller-only UID/GID mapping preflight; no guarantee of isolation |
| OCI lifecycle adapter via crun/runc | Development | internal/engine; real kernel runtime acceptance open |
| Read-only OCI image layout integrity verifier | Development | M2 preview; hashes referenced blobs, rejects external references and malformed layouts; no unpack/import/pull |
| Private local SHA-256 blob storage/import | Development | M2 preview; bounded, atomically linked opaque blobs, no extraction/pull or production store acceptance |
| Full OCI image store, layer unpack and registry support | Planned | M2: no production image pull/build |
| Rootless network, volumes and cgroups | Planned | M3 |
| Native GoreeCloud stack JSON validator, canonical digest and parallel-stage dry-run planner | Development | M4 preview; checks immutable image digests and restrictions; no workloads launched |
| Native stack executor and deployment reconciliation | Planned | M4; not merely a Docker Compose wrapper |
| Optional Docker Compose import/compatibility | Planned | M4, second priority to native stack |
| Optional Podman compatibility | Planned | M4, second priority |
| Daemon/API and service supervision | Planned | M5 |
| GoreeCloud Mesh, Identity, Shield, Wardveil and UI | Planned | M6 |
| VPS Mesh canary / operational default | Planned | M7–M8 |

Status vocabulary: **development** means code exists but is not accepted as stable; **planned** means not implemented. No feature above is asserted production-ready.
