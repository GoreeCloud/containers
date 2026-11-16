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

## Native stack declaration preview

The JSON-only stack schema is the **first-priority GoreeCloud definition format**. Its parser limits size/nesting, rejects duplicate keys, unknown fields, arbitrary host ports/mounts, mutable image references, root privileges, non-isolated networking, cycles and unbounded resource requests. Its plan is a **read-only proposal**, not a deployment action or policy enforcement at runtime. Secret references remain unresolved identifiers and must later integrate with GoreeCloud Identity, GoreeShield and Wardveil policy. No remote admission, storage, image fetch, runtime execution or Compose parsing is implemented here.

## Deterministic stage planning

Native stack plans now group independent services into dependency-safe parallel stages and include a canonical SHA-256 declaration fingerprint. These are deterministic, read-only planning artifacts, not an authorization or implementation of concurrent deployment. Stage dependencies and privacy controls must be revalidated at actual apply time; a matching fingerprint does not guarantee registry provenance or safe execution.

## Proposed stack transitions (development only)

The new stack diff OLD_JSON NEW_JSON command generates a deterministic, read-only change proposal. It orders additions by target dependency and removals in reverse original dependency order; any add, replace or removal requires manual review and future data/rollback acceptance. This **does not perform any migration** or imply that native deployment execution, rollback, secret resolution, network isolation, storage preservation or production admission is implemented.
