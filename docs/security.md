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

## Explicit read-only host capacity budgeting

The native stack capacity command accepts **caller-provided** CPU millicore, memory MiB, and max-instance envelopes; it computes declared aggregate demand, returns structured budget blockers and fails nonzero on oversubscription. It does not query live host resources, reserve cgroups, enforce limits, verify identities, resolve secrets, or authorize deployment; even an in-budget report declares deploymentAuthorized=false and requires manual review. Future real admission must inspect current host reservations, verified isolation and trusted MIPS policies before any atomic deployment.

## Linux rootless host capability observation

The optional host inspect command reads only fixed, size-bounded Linux proc/sysfs capability files to report the presence of cgroup v2 cpu/memory/pids controllers and unprivileged user-namespace configuration. It does not reveal usernames, paths, container names or secret data. It never invokes unshare, writes cgroup files, modifies services, verifies per-user cgroup delegation or proves namespace isolation. All reports explicitly retain deploymentAuthorized=false, delegationVerified=false and isolationVerified=false, even when every observed kernel fact appears favorable. VPS Mesh readiness must be verified separately on an authorized staging target.

## Immutable-in-memory native stack-file reads

The stack declaration loader now resolves an absolute local file parent, rejects symlinked parent directories and symlinked declaration files, opens via an os.Root directory descriptor, compares the opened inode to the checked file and requires exact bounded content length. The parsed document is a single in-memory snapshot; **this is not a signed policy decision or protection against a malicious same-user process modifying the opened inode concurrently**. Deployment remains unimplemented.

## Dependency-aware native stack risk impact

The read-only stack impact command compares two validated native specifications and tracks transitive dependents of changed services across both original and target dependency graphs. It computes a conservative **old-plus-new** potential overlapping CPU/memory envelope for change scenarios. This is a theoretical upper bound, not observed host capacity, actual coexistence scheduling, or a transactional rollback plan. Every changed scenario remains deploymentAuthorized=false, rollbackVerified=false and manualReviewRequired=true. It never deploys, stops or restarts a service.

## Explicit native transition overlap budget preflight

The native transition-capacity command compares both old and target stack specifications with caller-supplied CPU, memory and maximum instance budgets. It conservatively assumes the two deployments may coexist if any service changes, returns nonzero on exceeded declared limits, and never marks deployment authorized or rollback verified. Even a no-op must fit its existing declared budget. **These are not live host reservations, health checks, staging admission, resource enforcement or rollback guarantees**; all changes still require operator review.

## Offline stack revision conflict preflight

The native revision-check command requires the caller to supply the expected canonical SHA-256 fingerprint of a prior stack declaration. It fails with a structured conflict report if the supplied source document does not match that fingerprint; both matched and mismatched cases explicitly report deploymentAuthorized=false, liveStateVerified=false and rollbackVerified=false. This detects stale **input documents**, not concurrent changes to running workloads. A future privileged/authorized controller must atomically compare its actual committed generation under its own lock before any rollout.

### Single-source native preflight

The read-only native stack preflight command parses the source and target declarations once and evaluates a prior canonical SHA-256 fingerprint, old/new resource overlap and downstream dependency impact on the same in-memory values. Stale source fingerprints or over-budget declarations fail with a structured report. Even when both input checks pass, the report has deploymentAuthorized=false, liveStateVerified=false and rollbackVerified=false. The CLI never contacts running workloads or modifies an application.

### MIPS-sensitive secret-reference change gate

The source-only native preflight counts (but does not name or resolve) services whose secret references are introduced, removed or changed. It separately flags GoreeCloud Identity (authorization), GoreeShield (privacy and consent), and Wardveil (cybersecurity policy) for review; it explicitly reports MIPS authorization as unverified and exits unsuccessfully for sensitive changes. This is an offline safety gate, not an implemented policy provider, grant mechanism or proof of approved deployment.

### Fail-closed OCI image-reference changes in native stack preflight

An added/removed service or changed immutable image digest now requires separate Wardveil source/provenance and GoreeShield privacy reviews, recorded as unverified offline gates. The report includes only the affected service count, never the image digest or registry name. An in-budget, correct-revision declaration with changed images still fails preflight. This does NOT query the image store, verify a signed manifest, grant an approval or authorize a rollout.

The combined native preflight masks image-digest identifiers in its embedded transition-impact steps, while retaining action and service labels for review. Direct stack diff/impact commands remain separate developer diagnostics and must not be treated as privacy-redacted approval records.

### Operator grant is a required native transition gate

Even a source-valid, in-budget change to replicas, dependencies or other nonsecret application fields now fails the offline native preflight until a future GoreeCloud Identity operator authorization exists. The source-only tool reports goreeCloudIdentityReviewRequired=true and mipsAuthorizationVerified=false; it never grants, fetches or assumes an Identity token. A no-op input may pass offline checks but still cannot authorize a deployment.
