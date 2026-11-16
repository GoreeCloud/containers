# GoreeCloud Containers — Feature status

| Capability | Status | Evidence / limitation |
| --- | --- | --- |
| First-party Go command-line prototype | Development | cmd/gcontainers; Linux CI |
| OCI bundle policy validation | Development | internal/bundle; no guarantee of isolation |
| OCI lifecycle adapter via crun/runc | Development | internal/engine; real kernel runtime acceptance open |
| Read-only OCI image layout integrity verifier | Development | M2 preview; hashes referenced blobs, rejects external references and malformed layouts; no unpack/import/pull |
| Nonmutating local image-store audit | Development | Re-verifies committed blob bytes and rejects invalid names, links and oversized aggregate storage; not authenticated image provenance |
| Aggregate committed OCI content quota for cooperating importers | Development | Private serialized writer, 1-GiB total committed bytes; advisory and bypassable by direct writes |
| Private local SHA-256 blob storage/import | Development | M2 preview; bounded, atomically linked opaque blobs, no extraction/pull or production store acceptance |
| Read-only bounded tar/gzip layer safety preflight | Development | M2 preview; fail closed on links, traversal, device nodes and unsupported formats; no extraction |
| Digest-pinned offline OCI layer structural verification | Development | Single-pass SHA256 + bounded archive subset; no signature/unpack/run |
| Full OCI image store, layer unpack and registry support | Planned | M2: no production image pull/build |
| Rootless network, volumes and cgroups | Planned | M3 |
| Native GoreeCloud stack format and runner | Planned | M4; not merely a Docker Compose wrapper |
| Optional Docker Compose import/compatibility | Planned | M4, second priority to native stack |
| Optional Podman compatibility | Planned | M4, second priority |
| Daemon/API and service supervision | Planned | M5 |
| GoreeCloud Mesh, Identity, Shield, Wardveil and UI | Planned | M6 |
| VPS Mesh canary / operational default | Planned | M7–M8 |

Status vocabulary: **development** means code exists but is not accepted as stable; **planned** means not implemented. No feature above is asserted production-ready.
