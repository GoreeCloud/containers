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
- A deny-by-default seccomp action does **not** guarantee a safe profile: allowed syscalls must be reviewed. The current preview does not verify all Linux security settings of the low-level runtime or host.
- Rootless namespaces and UID mappings may be unsupported or configured differently on hosts. Host LSM (AppArmor/SELinux), cgroup delegation, network policies and real capability bounding are not end-to-end tested.
- The Go preview profile covers only a restricted subset of OCI config fields; complex bundles are intentionally rejected.
- No image provenance, signatures, vulnerability scanning, SBOM, registry authentication, secret injection, runtime event logs, multiuser authorization or recovery exercises.
- Process output from the OCI runtime is returned for successful `state` only; sensitive OCI runtime error output is not surfaced. Operator logs, when added, must follow privacy/data-minimization controls.
- No assertion of OCI conformance, kernel isolation acceptance, host hardening or Wardveil review.

## Acceptance before deployment

Dedicated rootless test host; kernel namespace/cgroup/LSM/seccomp experiments; malicious-bundle tests including symlink races, mount/device escape, UID mapping collisions and hostile rootfs; API threat model; dependency SBOM and vulnerability review; registry extraction/verification; credential scoping; backup and restore; resource exhaustion; failure injection and rollback. Independently review against GoreeShield and Wardveil requirements.

## Image layout verification preview

The read-only OCI layout verifier validates a bounded subset of OCI layout/index/manifest descriptor chains with SHA-256 and length checks. It refuses symlinks at directly checked paths, unsupported external URLs, oversized metadata, excessive blob count/depth, and malformed/duplicate-key JSON. **It does not unpack layers, pull images, establish signatures, prevent concurrent path replacement (TOCTOU), or certify arbitrary images as safe.** Registry operations, signed provenance, secure layer extraction, resource reservations and independent hostile-image review remain unimplemented.

## Experimental private blob store

The development branch adds a user-owned private local SHA-256 blob store. New bytes are streamed to a private temporary file, bounded to 512 MiB per blob, synced, hashed, and hard-linked into a digest-named path without overwriting existing blobs. Existing blobs are verified before deduplication. Explicit CLI import requires --experimental and an unprivileged Linux user. This store is **not** a secure container registry, full OCI image store, signed supply chain or artifact extraction pipeline. Bind mounts, malicious concurrent same-user modification, quotas across multiple blobs, disk exhaustion, crash recovery and LSM enforcement remain review gates. Linux openat2 descriptor-safe traversal should be evaluated before production.

## Confinement improvements

Local store operations now use an os.Root directory handle to anchor temporary writes, atomic links, and blob reads. Directory ownership/private permissions are verified, and inode identity is compared between metadata and open descriptors. Go's os.Root prevents traversal outside the anchored directory but **does not block bind mounts, device files, malicious same-user alteration, or all races**, so storage remains experimental. Linux descriptor, mount and privilege threat testing remains required.

## Read-only layer preflight

The development layer inspector parses a restricted tar or gzip-compressed tar subset with explicit entry count, input size and total uncompressed size limits. It rejects traversal, absolute paths, hard/symbolic links, device nodes, special permissions, duplicated or conflicting paths, whiteouts and PAX metadata. This **does not unpack, apply OCI whiteouts, support zstd, verify registry provenance or certify images as safe**. Unsupported valid OCI features intentionally fail closed pending a secure extractor, audited semantics, and isolated host acceptance.

## OCI layout descriptor-anchored verification

The read-only verifier now opens the OCI layout through a directory handle (os.Root) and rejects symlinked layout roots, blob directories, metadata and blob entries. It checks opened file identity against the file it validated, and snapshots index/manifest bytes **while hashing those same bytes** rather than reopening a mutable manifest path. This reduces symlink and between-read TOCTOU exposures; it does **not** defeat bind mounts, concurrent mutation of an opened regular file by the same account, or supply-chain signature forgery. OCI verification is integrity checking, not malware scanning or permission to execute.
