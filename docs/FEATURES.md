# GoreeCloud Containers — Feature status

| Capability | Status | Evidence / limitation |
| --- | --- | --- |
| First-party Go command-line prototype | Development | cmd/gcontainers; Linux CI |
| OCI bundle policy validation | Development | internal/bundle; strict process bounds and caller-only UID/GID mapping and reviewed external runtime version preflight; no guarantee of isolation |
| Read-only OCI incident census | Development | Private per-operation aggregate counts; nonzero on unresolved mutation; no process state attestation |
| Inert first-party kernel namespace probe | Development | User/mount/PID/IPC/UTS/net separation observation, not OCI confinement or deployment acceptance |
| OCI lifecycle adapter via crun/runc | Development | internal/engine; cooperative cross-process per-ID locking; real kernel runtime acceptance open |
| Read-only OCI image layout integrity verifier | Development | M2 preview; hashes referenced blobs, rejects external references and malformed layouts; no unpack/import/pull |
| Nonmutating local image-store audit | Development | Re-verifies committed blob bytes and rejects invalid names, links and oversized aggregate storage; not authenticated image provenance |
| Aggregate committed OCI content quota for cooperating importers | Development | Private serialized writer, 1-GiB total committed bytes; advisory and bypassable by direct writes |
| Private local SHA-256 blob storage/import | Development | M2 preview; bounded, atomically linked opaque blobs, no extraction/pull or production store acceptance |
| Read-only bounded tar/gzip layer safety preflight | Development | M2 preview; fail closed on links, traversal, device nodes and unsupported formats; no extraction |
| Full OCI image store, layer unpack and registry support | Planned | M2: no production image pull/build |
| Read-only Linux cgroup/userns prerequisites inspection | Development | Fixed allowlist of proc/sysfs facts; never verifies delegation or permits deployment |
| Rootless network, volumes and cgroups | Planned | M3 |
| Native GoreeCloud stack JSON validator, canonical digest, parallel-stage and side-effect-free transition/diff and dependency-impact risk planner, plus explicit host capacity budget review | Development | M4 preview; checks immutable image digests and restrictions; no workloads launched |
| Combined native stack source preflight | Development | Same parsed documents for SHA revision, declared old/new budget and dependency impact; never approves deployment |
| Source-declaration revision conflict preflight | Development | Requires caller-supplied prior canonical fingerprint; never verifies running state or authorizes deployment |
| Offline native transition-capacity overlap preflight | Development | Fails closed on CPU/memory/instance budget overcommit, never authorizes a rollout |
| Native stack executor and deployment reconciliation | Planned | M4; not merely a Docker Compose wrapper |
| Optional Docker Compose import/compatibility | Planned | M4, second priority to native stack |
| Optional Podman compatibility | Planned | M4, second priority |
| Daemon/API and service supervision | Planned | M5 |
| GoreeCloud Mesh, Identity, Shield, Wardveil and UI | Planned | M6 |
| VPS Mesh canary / operational default | Planned | M7–M8 |

Status vocabulary: **development** means code exists but is not accepted as stable; **planned** means not implemented. No feature above is asserted production-ready.
