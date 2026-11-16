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

Current upstream advisory review sets a **conservative development minimum** of crun 1.30.1 (the upstream 2026-09-25 security fix release) or runc 1.5.3 (upstream 2026-11-12 security patch release). Experimental OCI execution fails closed if a runtime reports a lower or ambiguous/non-stable upstream version, or lacks a trusted root-owned executable path. The doctor command reports baseline-eligible status separately from mere installation. This check is a **version preflight, not a proof of supply-chain provenance, secure vendor backports, host safety, or immunity to future CVEs**. An older distribution package may have backported fixes but requires independent verified distro security evidence before an exception; none has been implemented. Local laptop crun 1.14.1 is **not accepted** and has not been used to start containers.

References checked 2026-11-16: https://github.com/containers/crun/releases ; https://ubuntu.com/security/CVE-2026-88265 ; https://ubuntu.com/security/CVE-2026-84042 ; https://github.com/opencontainers/runc/releases . Re-check security notices before any runtime test, release or production acceptance.
