# Security model — 0.1 development preview

**Unreviewed experimental code. Not suitable for production workloads.**

## Implemented guardrails

1. OCI lifecycle commands refuse root execution, require Linux and an owned `0700` `XDG_RUNTIME_DIR` state root.
2. A rootless OCI low-level runtime (`crun` or `runc`) is an explicit dependency; it is never installed or downloaded automatically.
3. No shell invocation or arbitrary runtime binary names. Container IDs have a restrictive format; delete does not use `--force`.
4. Preview bundle loader rejects symlinked `config.json`, JSON duplicate keys, unknown fields, nesting above 64, and config size above 1 MiB.
5. Only private OCI namespaces, no host namespace joins; nonzero host UID/GID mappings, no custom devices, hooks, nonempty capabilities, interactive TTY or writable rootfs.
6. Explicit `noNewPrivileges` and a deny-by-default seccomp action are required. Host bind mounts and writable/shared mount options are rejected.
7. No remote endpoint, telemetry, registry credential handling, secrets storage or background service has been created.

## Residual risks / unimplemented controls

- The OCI bundle/rootfs can change between validation and execution (TOCTOU). File-descriptor-based traversal, immutable snapshots and secure extraction are not yet implemented.
- A deny-by-default seccomp action and a limited preview dangerous-syscall denial list do **not** guarantee a safe profile: allowed syscalls must be reviewed. The current preview does not verify all Linux security settings of the low-level runtime or host.
- Rootless namespaces and UID mappings may be unsupported or configured differently on hosts. Host LSM (AppArmor/SELinux), cgroup delegation, network policies and real capability bounding are not end-to-end tested.
- The Go preview profile covers only a restricted subset of OCI config fields; complex bundles are intentionally rejected.
- No image provenance, signatures, vulnerability scanning, SBOM, registry authentication, secret injection, runtime event logs, multiuser authorization or recovery exercises.
- Process output from the OCI runtime is returned for successful `state` only; sensitive OCI runtime error output is not surfaced. Operator logs, when added, must follow privacy/data-minimization controls.
- No assertion of OCI conformance, kernel isolation acceptance, host hardening or Wardveil review.

## Acceptance before deployment

Dedicated rootless test host; kernel namespace/cgroup/LSM/seccomp experiments; malicious-bundle tests including symlink races, mount/device escape, UID mapping collisions and hostile rootfs; API threat model; dependency SBOM and vulnerability review; registry extraction/verification; credential scoping; backup and restore; resource exhaustion; failure injection and rollback. Independently review against GoreeShield and Wardveil requirements.

## Incremental M1 hardening draft

The runtime preview now limits captured OCI runtime output to 64 KiB, restricts child process environment, rejects intersecting UID/GID maps and refuses unsupported mount parameters, seccomp flags, and a defined set of dangerous allowed syscalls. This is incomplete defense in depth, **not** Linux kernel isolation acceptance; full default-deny syscall allowlists, descriptor-safe bundle snapshots, subprocess lifecycle recovery, and LSM/cgroup verification remain required.

## M1 caller mapping and executable preflight

The development adapter now refuses executing OCI bundles whose container UID/GID 0 are not mapped **exactly and exclusively** to the invoking nonroot host user and primary group (one mapping of size 1 each). It also bounds process arguments and environment metadata, requires clean absolute executable and working-directory paths, and rejects duplicate or invalid environment names. Before CLI execution it requires a root-owned, non-group/world-writable crun or runc binary in an approved system directory. This is a conservative restriction, not proof that the binary, its dependencies, its distribution package, or the host kernel are trusted. Multiuser subordinate UID/GID ranges, writable mounts, privileged containers, general OCI compatibility, immutable bundle snapshots and actual rootless kernel confinement remain unsupported or unverified.

## Image layout verification preview

The read-only OCI layout verifier validates a bounded subset of OCI layout/index/manifest descriptor chains with SHA-256 and length checks. It refuses symlinks at directly checked paths, unsupported external URLs, oversized metadata, excessive blob count/depth, and malformed/duplicate-key JSON. **It does not unpack layers, pull images, establish signatures, prevent concurrent path replacement (TOCTOU), or certify arbitrary images as safe.** Registry operations, signed provenance, secure layer extraction, resource reservations and independent hostile-image review remain unimplemented.

## Experimental private blob store

The development branch adds a user-owned private local SHA-256 blob store. New bytes are streamed to a private temporary file, bounded to 512 MiB per blob, synced, hashed, and hard-linked into a digest-named path without overwriting existing blobs. Existing blobs are verified before deduplication. Explicit CLI import requires --experimental and an unprivileged Linux user. This store is **not** a secure container registry, full OCI image store, signed supply chain or artifact extraction pipeline. Bind mounts, malicious concurrent same-user modification, quotas across multiple blobs, disk exhaustion, crash recovery and LSM enforcement remain review gates. Linux openat2 descriptor-safe traversal should be evaluated before production.

## Confinement improvements

Local store operations now use an os.Root directory handle to anchor temporary writes, atomic links, and blob reads. Directory ownership/private permissions are verified, and inode identity is compared between metadata and open descriptors. Go's os.Root prevents traversal outside the anchored directory but **does not block bind mounts, device files, malicious same-user alteration, or all races**, so storage remains experimental. Linux descriptor, mount and privilege threat testing remains required.

## Native stack declaration preview

The JSON-only stack schema is the **first-priority GoreeCloud definition format**. Its parser limits size/nesting, rejects duplicate keys, unknown fields, arbitrary host ports/mounts, mutable image references, root privileges, non-isolated networking, cycles and unbounded resource requests. Its plan is a **read-only proposal**, not a deployment action or policy enforcement at runtime. Secret references remain unresolved identifiers and must later integrate with GoreeCloud Identity, GoreeShield and Wardveil policy. No remote admission, storage, image fetch, runtime execution or Compose parsing is implemented here.

## Deterministic stage planning

Native stack plans now group independent services into dependency-safe parallel stages and include a canonical SHA-256 declaration fingerprint. These are deterministic, read-only planning artifacts, not an authorization or implementation of concurrent deployment. Stage dependencies and privacy controls must be revalidated at actual apply time; a matching fingerprint does not guarantee registry provenance or safe execution.

## External runtime security-version preflight (2026-11-16)

Current upstream advisory review sets a **conservative development minimum** of crun 1.31 (the upstream 2026-11-12 security fix release) or runc 1.5.3 (upstream 2026-11-12 security patch release). Experimental OCI execution fails closed if a runtime reports a lower or ambiguous/non-stable upstream version, or lacks a trusted root-owned executable path. The doctor command reports baseline-eligible status separately from mere installation. This check is a **version preflight, not a proof of supply-chain provenance, secure vendor backports, host safety, or immunity to future CVEs**. An older distribution package may have backported fixes but requires independent verified distro security evidence before an exception; none has been implemented. Local laptop crun 1.14.1 is **not accepted** and has not been used to start containers.

References checked 2026-11-16: https://github.com/containers/crun/security/advisories/GHSA-jj7h-4c3m-j6wf ; https://github.com/containers/crun/security/advisories/GHSA-2mxx-58qc-p466 ; https://github.com/containers/crun/releases ; https://ubuntu.com/security/CVE-2026-88265 ; https://ubuntu.com/security/CVE-2026-84042 ; https://github.com/opencontainers/runc/releases . Re-check security notices before any runtime test, release or production acceptance.

## OCI subprocess cancellation

The low-level runtime adapter now places its child invocation in an isolated process group, sends SIGKILL to that group on context cancellation, and pins a trusted search PATH rather than inheriting the invoking shell PATH. Timeouts are tested with a synthetic system sleep subprocess. This is a bounded shutdown safeguard, not a full service supervisor or guarantee that processes that leave the group, escaped descendants, or already-registered OCI runtime containers are terminated. Persistent state reconciliation and independent termination acceptance remain required.

## Read-only layer preflight

The development layer inspector parses a restricted tar or gzip-compressed tar subset with explicit entry count, input size and total uncompressed size limits. It rejects traversal, absolute paths, hard/symbolic links, device nodes, special permissions, duplicated or conflicting paths, whiteouts and PAX metadata. This **does not unpack, apply OCI whiteouts, support zstd, verify registry provenance or certify images as safe**. Unsupported valid OCI features intentionally fail closed pending a secure extractor, audited semantics, and isolated host acceptance.

## Proposed stack transitions (development only)

The new stack diff OLD_JSON NEW_JSON command generates a deterministic, read-only change proposal. It orders additions by target dependency and removals in reverse original dependency order; any add, replace or removal requires manual review and future data/rollback acceptance. This **does not perform any migration** or imply that native deployment execution, rollback, secret resolution, network isolation, storage preservation or production admission is implemented.

## Fail-closed OCI lifecycle state handling

Read-only OCI state responses now require a matching strict container ID, an accepted stable state, valid running PID, bounded output, and duplicate-key-safe JSON. Returned state is minimized to ID/status/PID; upstream annotations and bundle paths are never echoed. Start is allowed only from created, stop only from running, and non-force delete only from created/stopped. These checks reduce accidental transitions and privacy leakage; **they do not prevent state changes racing between read and execution, do not implement durable lock arbitration or reconciliation, and are not production state-management acceptance**.

## OCI layout descriptor-anchored verification

The read-only verifier now opens the OCI layout through a directory handle (os.Root) and rejects symlinked layout roots, blob directories, metadata and blob entries. It checks opened file identity against the file it validated, and snapshots index/manifest bytes **while hashing those same bytes** rather than reopening a mutable manifest path. This reduces symlink and between-read TOCTOU exposures; it does **not** defeat bind mounts, concurrent mutation of an opened regular file by the same account, or supply-chain signature forgery. OCI verification is integrity checking, not malware scanning or permission to execute.

## Explicit read-only host capacity budgeting

The native stack capacity command accepts **caller-provided** CPU millicore, memory MiB, and max-instance envelopes; it computes declared aggregate demand, returns structured budget blockers and fails nonzero on oversubscription. It does not query live host resources, reserve cgroups, enforce limits, verify identities, resolve secrets, or authorize deployment; even an in-budget report declares deploymentAuthorized=false and requires manual review. Future real admission must inspect current host reservations, verified isolation and trusted MIPS policies before any atomic deployment.

## OCI bundle descriptor anchoring

Bundle policy parsing now opens an OCI bundle through an os.Root directory handle, rejects a symlinked bundle path, symlinked config entries and symlinks in the declared rootfs directory path. It cross-checks opened config file identity and reads bounded JSON from that same descriptor. **The external OCI runtime still independently reopens paths after validation**; this is a preview TOCTOU reduction, not immutable snapshotting or authorization for untrusted bundle execution. Test rootfs content, file mutation after checks, LSM confinement and failure recovery before runtime acceptance.

## Bounded compressed-layer stream semantics

OCI tar/gzip inspection now hard-limits compressed input with a reader that **errors** on overflow rather than silently truncating the source, and treats gzip as exactly one member. Trailing compressed bytes and concatenated gzip members are rejected after fully draining/verifying the decompressor checksum. These constraints deliberately reject some inputs that other OCI tools might accept; secure extraction, registry provenance and live image mounting remain unimplemented.

## Linux rootless host capability observation

The optional host inspect command reads only fixed, size-bounded Linux proc/sysfs capability files to report the presence of cgroup v2 cpu/memory/pids controllers and unprivileged user-namespace configuration. It does not reveal usernames, paths, container names or secret data. It never invokes unshare, writes cgroup files, modifies services, verifies per-user cgroup delegation or proves namespace isolation. All reports explicitly retain deploymentAuthorized=false, delegationVerified=false and isolationVerified=false, even when every observed kernel fact appears favorable. VPS Mesh readiness must be verified separately on an authorized staging target.

## Cross-process cooperative OCI mutation locking

The real low-level runtime adapter now wraps create/start/stop/delete and their state checks in an exclusive per-container lock under a user-owned private runtime directory. Locks are descriptor-anchored, no-follow, permission/owner/inode checked and honor caller cancellation without blocking indefinitely. This prevents conflicting GoreeCloud CLI mutations from running concurrently for the same ID, while independent IDs can proceed concurrently. The lock is **cooperative only**: direct crun/runc invocations, non-GoreeCloud agents, changes to the rootfs after validation, crashes during a runtime operation and runtime state recovery are NOT covered. Tests use synthetic operations only; no OCI container was launched. Avoid treating this as crash-safe or a production admission guarantee.

## Local content-store integrity auditing

The read-only image audit command opens an **existing** private user-owned blob store without creating or repairing it. It verifies the SHA-256 bytes of each committed regular file, rejects symlinks, unexpected names, hardlinks, oversized entries, more than 4096 entries, and aggregate audit sizes above 1 GiB. The summary intentionally omits sensitive paths and filenames. A passing audit proves observed byte integrity only, not provenance, signatures, malware safety, registry trust, or protection against same-user mutations after audit. Aggregate limits are verification bounds, **not disk reservations or enforcement of a global write quota**.

## Immutable-in-memory native stack-file reads

The stack declaration loader now resolves an absolute local file parent, rejects symlinked parent directories and symlinked declaration files, opens via an os.Root directory descriptor, compares the opened inode to the checked file and requires exact bounded content length. The parsed document is a single in-memory snapshot; **this is not a signed policy decision or protection against a malicious same-user process modifying the opened inode concurrently**. Deployment remains unimplemented.

## Durable pending mutation records — fail-closed recovery preview

The experimental real-runner adapter now creates a private, synchronized, exclusive per-ID pending journal marker before invoking any OCI create/start/stop/delete operation, while holding its cooperative per-container lock. The marker is cleared only after the runtime reports success and a directory sync succeeds. On a runtime error, interruption or crash, a pending marker remains and the next cooperating mutation is **blocked until a separate audited recovery process resolves it**. The read-only OCI pending command reads a redacted minimal incident status from an existing user-private directory without contacting the OCI runtime or deleting markers. No automatic cleanup, automatic retry, or unchecked manual reset was introduced. A cleared marker alone is **not proof that the container is healthy**; direct external runtimes and malicious same-user edits are not covered, and proper crash recovery remains an unmet Stable acceptance gate.

## Detection of interrupted or active import staging

Read-only image-store audits now also inventory bounded user-private temporary ingestion entries, reporting only counts and total staging bytes, never names or paths. A staging file may be actively written by another process or left after an interrupted import; either requires operator review. The audit rejects symlinks, unusual names, unsafe ownership, hardlinks and exceeded inspection limits but **does not delete, repair or garbage-collect content**. Reported storage totals are observational, not disk quotas or evidence of clean crash recovery.

When any active or interrupted import staging entry exists, the aggregate audit deliberately reports verified=false and operatorReviewRequired=true. No unattended cleanup or false healthy status is permitted.

## Dependency-aware native stack risk impact

The read-only stack impact command compares two validated native specifications and tracks transitive dependents of changed services across both original and target dependency graphs. It computes a conservative **old-plus-new** potential overlapping CPU/memory envelope for change scenarios. This is a theoretical upper bound, not observed host capacity, actual coexistence scheduling, or a transactional rollback plan. Every changed scenario remains deploymentAuthorized=false, rollbackVerified=false and manualReviewRequired=true. It never deploys, stops or restarts a service.

## Privacy-minimized runtime incident census

The read-only pending-summary CLI inventories a bounded private journal, rejects symlinks, unknown entries and malformed intent records, and reports only aggregate counts of incomplete create/start/stop/delete operations. It does **not expose container identifiers, process PIDs, bundle paths or secrets**. An unresolved intent produces a nonzero exit status and requires operator review. It does not query real OCI processes, detect journal entries removed externally, or repair runtime state. Summaries may change during concurrent activity and cannot be treated as stable snapshots.

## Advisory aggregate committed-blob quota

Cooperating OCI blob imports now take a private, no-follow, per-store exclusive filesystem lock before inspecting committed digest bytes and pending ingestion files. New unique commits are rejected when they would exceed a 1 GiB aggregate committed-byte preview quota; verified same-digest imports deduplicate without consuming more capacity. The lock is held through the temporary ingest and hardlink commit. This bounds committed content for cooperating import paths, but **does not reserve temporary-disk space**, cannot stop same-user direct filesystem writes or malicious bypasses, and cannot guarantee free host disk or safe eviction. Full store recovery, monitored filesystem capacity and actual quota enforcement remain Stable release gates.

## Explicit native transition overlap budget preflight

The native transition-capacity command compares both old and target stack specifications with caller-supplied CPU, memory and maximum instance budgets. It conservatively assumes the two deployments may coexist if any service changes, returns nonzero on exceeded declared limits, and never marks deployment authorized or rollback verified. Even a no-op must fit its existing declared budget. **These are not live host reservations, health checks, staging admission, resource enforcement or rollback guarantees**; all changes still require operator review.

The private journal census now compares two consecutive bounded, deterministic inventories (including redacted internal marker identities/operation kinds) and fails closed when they disagree. The internal comparison digest is deliberately never exposed. As with any observational read, a second successful pass does not lock or attest to external runtime state, and changes immediately afterward remain possible.

The OCI writer lock now rechecks its pathname inode **after** acquiring the advisory lock, catching common interrupted wait/unlink/replacement cases. Separate-process synthetic tests verify the same private store cannot admit a competing cooperating importer while the lock is held. A malicious same-user actor removing the lock **after** this check is still outside the security model and remains a hard filesystem ownership/isolation requirement.

## Offline stack revision conflict preflight

The native revision-check command requires the caller to supply the expected canonical SHA-256 fingerprint of a prior stack declaration. It fails with a structured conflict report if the supplied source document does not match that fingerprint; both matched and mismatched cases explicitly report deploymentAuthorized=false, liveStateVerified=false and rollbackVerified=false. This detects stale **input documents**, not concurrent changes to running workloads. A future privileged/authorized controller must atomically compare its actual committed generation under its own lock before any rollout.

### Fail-closed OCI state reads

The real adapter now acquires the per-container cooperative lock and checks the private pending journal BEFORE reading state from the external OCI runtime. An unresolved mutation causes a nonzero result instead of a misleading healthy status. Synthetic injected-runner tests verify that a pending intent blocks status reads and that a concurrent mutation lock delays status. This cannot attest to the state of processes manipulated directly by unrelated runtimes.

### Hardened individual blob verification

The private store Verify operation now rejects group/world-readable files, unexpected ownership, extra hardlinks, unsafe symlink paths, and changes in the opened file/path identity during verification. Bounded, cancellation-aware hashing detects content mismatch and unexpected growth. This closes deduplication checks that previously relied primarily on matching bytes; it remains a moment-in-time integrity check, not source authentication or resistance to hostile same-user modification.
