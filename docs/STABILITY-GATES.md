# GoreeCloud Containers — Stable acceptance gates

**Status:** experimental development review; **NOT STABLE, NOT DEPLOYED**. Last reviewed November 16, 2026. This is a release gate register, not an authorization to start workloads or a replacement for GoreeCloud's canonical Drive policies.

## Non-negotiable platform direction

GoreeCloud Containers is the intended **first-priority container platform when independently stable**. Docker, Docker Compose and Podman become optional second-priority alternatives, not dependencies of the native engine. Until each workload passes cutover tests, preserve working deployments and tested rollback. Never install new workloads on retiring VPS-01/VPS-02; stage future VPS changes only on an authorized isolated VPS Mesh target.

## Current evidence (development only)

| Area | Verified in development | Stable gate status |
| --- | --- | --- |
| Go CLI, original code, bounded OCI policy | CI, tests, vet, build, selected fuzz tests | **Incomplete** — security review and OS acceptance missing |
| OCI runtime security | Strict nonroot mapping/profile, patched version gate, minimal state parser, cooperative per-ID locks, private pending-operation journal with double-read incident consistency and fail-closed status reads | **BLOCKED** — no actual rootless kernel confinement or recovery test |
| OCI images | Bounded hash verification, private opaque blob store, read-only audit (including unresolved import staging and unsafe ownership/permissions), private SHA-256 Verify, cooperating 1-GiB committed-blob write quota, cross-process lock test and tar/gzip preflight | **BLOCKED** — no authenticated registry, secure unpacker or signature verification |
| Host resources | Read-only cgroup/userns capability probe; caller-supplied stack capacity math | **BLOCKED** — actual cgroup delegation and enforcement not implemented |
| Native stack | JSON validator, dependency stages, fingerprint, read-only transition diff, dependency blast-radius, explicit old-plus-new budget, caller-supplied revision conflict checks and combined single-source preflight | **BLOCKED** — executor, health checks and rollback not implemented |
| MIPS systems | Boundaries documented | **BLOCKED** — no implemented/proven platform integration |
| VPS Mesh staging | No production action from this review branch | **BLOCKED** — isolated canary, service parity, observability and recovery required |

### M1 — Runtime security gate

- [ ] Verified provenance of root-owned, patched low-level `crun`/`runc` binary; review upstream and distribution advisories again at test time. On the development laptop, the previously observed user-local `crun 1.31` is user-owned and not accepted; system `/usr/bin/crun 1.14.1` is below the current reviewed minimum.
- [ ] Unprivileged UID/GID mapping, user/mount/pid/network/IPC/UTS namespaces, seccomp/LSM confinement, mount immutability and cgroup behavior proved using isolated real workloads and adversarial tests.
- [ ] Eliminate execution-time bundle/rootfs path races through immutable snapshots or equivalent audited mechanisms. Current `os.Root` validation still hands a filesystem pathname to an external runtime.
- [ ] Recover from crashes and leaked processes; prevent conflicting operations by other tooling, not just cooperating GoreeCloud CLI invocations. Per-ID locks and a fail-closed pending marker provide visibility but **not repair or correctness guarantees**; automatic recovery is not implemented.

### M2 — Image provenance, unpacking and storage

- [ ] Registry authentication without exposed credentials, pinned manifests and signature/provenance controls reviewed by Wardveil and GoreeShield.
- [ ] Separate safe extraction implementation with OCI whiteout and permissions semantics, link/path/device restrictions, quotas, rollback, and hostile archive tests.
- [ ] Persistent content eviction and hard disk quota/temporary space enforcement tested. Current 1-GiB limit is enforced only across cooperating CLI import writers and can be bypassed by direct filesystem writes. A successful read-only audit verifies only the observed committed bytes; a pending import now forces a failing audit exit and operator review, not automatic deletion. It does not prove provenance, store immutability or real disk quotas.

### M3–M4 — Native operations before compatibility adapters

- [ ] Real rootless network isolation, default-deny service connectivity, private named volumes, cgroup CPU/memory/PID enforcement and restart/recovery testing.
- [ ] Native stack apply, health/readiness ordering, secrets via GoreeCloud Identity and GoreeShield, deployment locks, transactional updates and rollback.
- [ ] A future native executor must independently compare the authenticated **running** stack generation under its own exclusive lock; an offline source declaration fingerprint match is not live reconciliation or deployment consent.
- [ ] Optional Docker/Compose/Podman import and compatibility adapters after native core stability; no external engine must be required for native operations.

### M5–M8 — Platform and cutover

- [ ] Secure versioned local API, least-privilege authorization, telemetry without prohibited data collection, reliable supervision, and maintenance support.
- [ ] **GoreeCloud UI:** verified common design system and accessibility.
- [ ] **GoreeCloud Mesh:** verified integration and data-flow contracts.
- [ ] **GoreeCloud Identity:** authenticated, least-privilege operator and service access.
- [ ] **GoreeShield:** consent, data minimization, retention and privacy acceptance.
- [ ] **Wardveil:** cybersecurity policy, scanning, threat and incident response acceptance.
- [ ] Authorized VPS Mesh **staging** canary, documented recovery path, parity, load tests, backup verification, reversible per-workload cutover and operator sign-off. Legacy services must remain safe until validated completion.

## Evidence rules

Only mark a gate **passed** after tested implementation, exact commit, reproducible command/results, reviewer acceptance, and where relevant evidence on the actual target. GitHub CI, fuzzing and dry-run plans are useful evidence **but never substitute for a real isolated Linux test or production approval**. A passing version check does not establish package supply-chain provenance. No unpublished credential, private infrastructure address, or sensitive data belongs in public CI logs or this document.

**Source of implementation evidence:** [draft foundation PR #1](https://github.com/GoreeCloud/containers/pull/1), [runtime PR #7](https://github.com/GoreeCloud/containers/pull/7), [OCI image PR #8](https://github.com/GoreeCloud/containers/pull/8), [native stack PR #9](https://github.com/GoreeCloud/containers/pull/9), and [combined integration PR #10](https://github.com/GoreeCloud/containers/pull/10). The stable gate remains **not accepted** while these are drafts.
