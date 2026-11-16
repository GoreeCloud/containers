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
