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

## External runtime security-version preflight (2026-11-16)

Current upstream advisory review sets a **conservative development minimum** of crun 1.31 (the upstream 2026-11-12 security fix release) or runc 1.5.3 (upstream 2026-11-12 security patch release). Experimental OCI execution fails closed if a runtime reports a lower or ambiguous/non-stable upstream version, or lacks a trusted root-owned executable path. The doctor command reports baseline-eligible status separately from mere installation. This check is a **version preflight, not a proof of supply-chain provenance, secure vendor backports, host safety, or immunity to future CVEs**. An older distribution package may have backported fixes but requires independent verified distro security evidence before an exception; none has been implemented. Local laptop crun 1.14.1 is **not accepted** and has not been used to start containers.

References checked 2026-11-16: https://github.com/containers/crun/security/advisories/GHSA-jj7h-4c3m-j6wf ; https://github.com/containers/crun/security/advisories/GHSA-2mxx-58qc-p466 ; https://github.com/containers/crun/releases ; https://ubuntu.com/security/CVE-2026-88265 ; https://ubuntu.com/security/CVE-2026-84042 ; https://github.com/opencontainers/runc/releases . Re-check security notices before any runtime test, release or production acceptance.

## OCI subprocess cancellation

The low-level runtime adapter now places its child invocation in an isolated process group, sends SIGKILL to that group on context cancellation, and pins a trusted search PATH rather than inheriting the invoking shell PATH. Timeouts are tested with a synthetic system sleep subprocess. This is a bounded shutdown safeguard, not a full service supervisor or guarantee that processes that leave the group, escaped descendants, or already-registered OCI runtime containers are terminated. Persistent state reconciliation and independent termination acceptance remain required.

## Fail-closed OCI lifecycle state handling

Read-only OCI state responses now require a matching strict container ID, an accepted stable state, valid running PID, bounded output, and duplicate-key-safe JSON. Returned state is minimized to ID/status/PID; upstream annotations and bundle paths are never echoed. Start is allowed only from created, stop only from running, and non-force delete only from created/stopped. These checks reduce accidental transitions and privacy leakage; **they do not prevent state changes racing between read and execution, do not implement durable lock arbitration or reconciliation, and are not production state-management acceptance**.

## OCI bundle descriptor anchoring

Bundle policy parsing now opens an OCI bundle through an os.Root directory handle, rejects a symlinked bundle path, symlinked config entries and symlinks in the declared rootfs directory path. It cross-checks opened config file identity and reads bounded JSON from that same descriptor. **The external OCI runtime still independently reopens paths after validation**; this is a preview TOCTOU reduction, not immutable snapshotting or authorization for untrusted bundle execution. Test rootfs content, file mutation after checks, LSM confinement and failure recovery before runtime acceptance.

## Cross-process cooperative OCI mutation locking

The real low-level runtime adapter now wraps create/start/stop/delete and their state checks in an exclusive per-container lock under a user-owned private runtime directory. Locks are descriptor-anchored, no-follow, permission/owner/inode checked and honor caller cancellation without blocking indefinitely. This prevents conflicting GoreeCloud CLI mutations from running concurrently for the same ID, while independent IDs can proceed concurrently. The lock is **cooperative only**: direct crun/runc invocations, non-GoreeCloud agents, changes to the rootfs after validation, crashes during a runtime operation and runtime state recovery are NOT covered. Tests use synthetic operations only; no OCI container was launched. Avoid treating this as crash-safe or a production admission guarantee.

## Durable pending mutation records — fail-closed recovery preview

The experimental real-runner adapter now creates a private, synchronized, exclusive per-ID pending journal marker before invoking any OCI create/start/stop/delete operation, while holding its cooperative per-container lock. The marker is cleared only after the runtime reports success and a directory sync succeeds. On a runtime error, interruption or crash, a pending marker remains and the next cooperating mutation is **blocked until a separate audited recovery process resolves it**. The read-only OCI pending command reads a redacted minimal incident status from an existing user-private directory without contacting the OCI runtime or deleting markers. No automatic cleanup, automatic retry, or unchecked manual reset was introduced. A cleared marker alone is **not proof that the container is healthy**; direct external runtimes and malicious same-user edits are not covered, and proper crash recovery remains an unmet Stable acceptance gate.

## Privacy-minimized runtime incident census

The read-only pending-summary CLI inventories a bounded private journal, rejects symlinks, unknown entries and malformed intent records, and reports only aggregate counts of incomplete create/start/stop/delete operations. It does **not expose container identifiers, process PIDs, bundle paths or secrets**. An unresolved intent produces a nonzero exit status and requires operator review. It does not query real OCI processes, detect journal entries removed externally, or repair runtime state. Summaries may change during concurrent activity and cannot be treated as stable snapshots.

The private journal census now compares two consecutive bounded, deterministic inventories (including redacted internal marker identities/operation kinds) and fails closed when they disagree. The internal comparison digest is deliberately never exposed. As with any observational read, a second successful pass does not lock or attest to external runtime state, and changes immediately afterward remain possible.

### Fail-closed OCI state reads

The real adapter now acquires the per-container cooperative lock and checks the private pending journal BEFORE reading state from the external OCI runtime. An unresolved mutation causes a nonzero result instead of a misleading healthy status. Synthetic injected-runner tests verify that a pending intent blocks status reads and that a concurrent mutation lock delays status. This cannot attest to the state of processes manipulated directly by unrelated runtimes.

### Post-acquisition OCI lock identity check

The private per-container cooperative lock now checks its pathname inode, owner and private permissions **after** acquiring the filesystem flock, not only before waiting. This catches a replaced lock pathname during a competing operation's wait. Direct OCI runtime invocations, hostile same-user lock replacement after the check and real kernel process supervision remain unverified and cannot be treated as solved.

### Inert first-party kernel namespace probe

The new gcontainers kernel-probe command creates only an unprivileged copy of its own first-party executable with isolated user, mount, PID, IPC, UTS and network namespace flags and single-caller UID/GID mapping. It compares namespace identities and reports only booleans, not inode identifiers, PIDs, or secret values. No rootfs is mounted and no OCI binary, application, image, networking, or cgroup is configured. Passing namespace creation NEVER verifies OCI isolation, cgroup delegation, seccomp, LSM or runtime provenance and cannot authorize a deployment. A denied clone is reported as unverified rather than bypassed.

The kernel probe now executes its own already-running executable through the Linux /proc/self/exe inode instead of reopening a replaceable user-writable executable pathname. The inert child receives the parent namespace IDs on a bounded stdin pipe and returns only booleans; even the internal helper no longer prints raw namespace inode identities. This is a reduction in probe tampering and diagnostic disclosure risk, not OCI confinement.
