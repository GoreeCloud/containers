# GoreeCloud Containers — Feature status

| Capability | Status | Evidence / limitation |
| --- | --- | --- |
| First-party Go command-line prototype | Development | cmd/gcontainers; Linux CI |
| OCI bundle policy validation | Development | internal/bundle; no guarantee of isolation |
| OCI lifecycle adapter via crun/runc | Development | internal/engine; real kernel runtime acceptance open |
| Rootless image store and OCI registry support | Planned | M2: no production image pull/build |
| Read-only Linux cgroup/userns prerequisites inspection | Development | Fixed allowlist of proc/sysfs facts; never verifies delegation or permits deployment |
| Rootless network, volumes and cgroups | Planned | M3 |
| Native GoreeCloud stack JSON validator, canonical digest, parallel-stage and side-effect-free transition/diff and dependency-impact risk planner, plus explicit host capacity budget review | Development | M4 preview; checks immutable image digests and restrictions; no workloads launched |
| Source-declaration revision conflict preflight | Development | Requires caller-supplied prior canonical fingerprint; never verifies running state or authorizes deployment |
| Offline native transition-capacity overlap preflight | Development | Fails closed on CPU/memory/instance budget overcommit, never authorizes a rollout |
| Native stack executor and deployment reconciliation | Planned | M4; not merely a Docker Compose wrapper |
| Optional Docker Compose import/compatibility | Planned | M4, second priority to native stack |
| Optional Podman compatibility | Planned | M4, second priority |
| Daemon/API and service supervision | Planned | M5 |
| GoreeCloud Mesh, Identity, Shield, Wardveil and UI | Planned | M6 |
| VPS Mesh canary / operational default | Planned | M7–M8 |

Status vocabulary: **development** means code exists but is not accepted as stable; **planned** means not implemented. No feature above is asserted production-ready.
