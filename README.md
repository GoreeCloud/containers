# GoreeCloud Containers

**An experimental, GoreeCloud-owned Linux container engine and management platform.**

GoreeCloud Containers is being built as the eventual default containerization solution for the GoreeCloud ecosystem, with a design goal of permanently replacing Docker and Podman as the default management engines after independently verified stability. Docker, Docker Compose, and Podman remain optional, second-priority compatibility paths; none is required by the intended stable GoreeCloud Containers engine.

> **Status: 0.1.0-dev / experimental foundation. Not production-ready.** A restricted rootless OCI lifecycle, private content store, digest-pinned HTTPS manifest/blob retrieval with verified cache reuse and inert ordered layer assembly, private volume directories and local read-only API are implemented. Executable OCI image preparation/build, authenticated registry and tag resolution, volume mounting, networking, daemon, Compose execution, GUI, remote control and cluster orchestration are not implemented. Do not migrate production containers.

## Implemented in this development branch

- `gcontainers doctor`: a read-only report of local Linux and OCI runtime prerequisites.
- `gcontainers validate <bundle-directory>`: fail-closed parsing of a restricted rootless OCI bundle policy. Includes file-size and JSON nesting limits, duplicate-key rejection, and restrictions on host namespaces, root filesystem, capabilities, devices, hooks, writable/bind mounts, and seccomp defaults.
- `gcontainers preflight <bundle-directory>`: inert, read-only policy and guest-layout inspection with redacted failure reasons. Checks the executable, required guest directories, ELF interpreter and bounded nested shebang chain; rejects `/usr/bin/env` and `/bin/env` secondary command resolution. It also rejects default-deny seccomp declarations without an explicit guest `execve` allowance (a necessary but not sufficient bootstrap condition). It **never invokes an OCI runtime** and explicitly returns `isolationVerified=false` and `deploymentAuthorized=false` even on a positive result.
- `gcontainers oci create|start|state|stop|delete`: a GoreeCloud-owned lifecycle adapter that invokes an external, replaceable OCI low-level runtime (`crun` or `runc`) directly, without Docker, Podman, containerd, a shell, or a background daemon. Mutations require `--experimental`; runtime state is stored in a private per-user directory and deletion is never forced and must verify exact runtime metadata absence before clearing the journal.
- OCI state reads strictly parse bounded runtime JSON and return only container ID, lifecycle state and PID. The reviewed `runc 1.5.3` response additionally includes string metadata `created`, `owner` and `rootfs`; these fields are recognized but **never displayed**, while unknown metadata, duplicate keys and mismatched IDs remain rejected. This compatibility allowance is not proof of successful OCI workload execution or release readiness.
- A journaled OCI `create` now performs an immediate strictly decoded `state` readback: a low-level runtime exit status 0 with a guest already `stopped` is **not** a successful container creation. In that case the pending journal is retained for operator recovery; passing this check does not guarantee the guest will remain alive or that subsequent `start` and confinement have been accepted.
- Additional preflight: the preview requires the exact invoking nonroot host UID/GID mappings and an approved root-owned system-installed low-level OCI runtime executable. No subordinate UID/GID ranges yet. Experimental execution now refuses external crun below 1.31 or runc below 1.5.3, and ambiguous versions; this is a conservative 2026-11-16 preflight, not a security certification.
- Read-only doctor reports crun_admission_status and runc_admission_status as not_installed, untrusted_executable, runtime_version_or_probe_rejected, or preview_preflight_passed. These are privacy-minimized preflight reasons, not deployment authorization; every real OCI operation still repeats independent runtime checks.
- Private local SHA-256 content-addressed blob store with bounded, directory-handle-anchored, atomic, deduplicated imports (requires explicit experimental confirmation; **blob import alone does not authorize execution**).
- Read-only OCI image-layout integrity verification: referenced SHA-256 blobs, descriptor sizes, metadata limits, no downloads or extraction. Supported OCI media types are intentionally restricted; **this is not an image store or container image engine**.
- Native GoreeCloud stack JSON preview: strict, versioned schema validation and dependency-ordered, parallel-stage **dry-run plans only**, with a deterministic SHA-256 specification fingerprint (an audit identifier, not an approval). No Docker Compose engine is required; no stack apply or service deployment exists yet.

- Read-only inspection of a restricted tar/tar+gzip OCI layer subset: rejects path traversal, links, devices, whiteouts, extended metadata, duplicate paths, dangerous modes, oversized entries and decompression abuse. No extraction or filesystem writes.
- Further development safeguards: cooperating cross-process runtime locks and private incident-journal census; read-only full-store SHA-256 audit with advisory aggregate import quota; descriptor-anchored stack declarations and offline old-plus-new resource budget checks. None replaces isolation, signed provenance or production authorization.
- Standard-library-only Go code, initial unit tests and Linux CI.

The execution adapter has passed narrow first-party isolated Linux OCI lifecycle tests, including all six namespace comparisons and kernel cgroup readback, but has not passed a comprehensive independent security audit or Stable acceptance. OCI runtimes remain required for execution; this is not a from-scratch kernel runtime. Validation reduces obvious hazards, but cannot certify an arbitrary OCI bundle as safe.

## Build and try the read-only commands

Requires Linux and Go 1.27 or later.

```sh
go test ./...
go build -o gcontainers ./cmd/gcontainers
./gcontainers doctor
# Read-only SHA256 verification of all committed blobs in a private store:
./gcontainers image audit --store /home/you/private-image-store
# Cooperating imports enforce an aggregate 1-GiB committed-blob quota;
# this is not a hard disk-space reservation or secure OCI extraction.
./gcontainers image verify /path/to/oci-layout
./gcontainers image inspect-layer /path/to/layer.tar tar
# Offline exact descriptor digest + hostile-archive structural preflight:
./gcontainers image verify-layer /absolute/local/layer.tar tar sha256:YOUR_VERIFIED_64_HEX_DIGEST
# When the caller has an OCI layer descriptor, also provide its exact compressed bytes:
./gcontainers image verify-layer /absolute/local/layer.tar tar sha256:YOUR_VERIFIED_64_HEX_DIGEST 10240
# Optional experimental local blob import (opaque bytes only):
./gcontainers image import --store /home/you/private-image-store --file /path/to/blob --experimental
# Read-only, non-identifying Linux kernel capabilities (not a security attestation):
./gcontainers host inspect
./gcontainers stack validate examples/native-stack.json
./gcontainers stack plan examples/native-stack.json
# Advisory reverse-dependency stop stages; no workloads stopped:
./gcontainers stack shutdown-plan examples/native-stack.json
# Advisory restart of a service and its transitive dependents:
./gcontainers stack restart-plan examples/native-stack.json db
# Offline, caller-supplied host budget; this is NOT a deployment authorization:
./gcontainers stack capacity examples/native-stack.json --cpu-milli 4000 --memory-mib 2048 --max-instances 16
# Risk-impact reports remain advisory; no resources are reserved or deployed.
# Read-only changes between two native declarations:
./gcontainers stack diff examples/native-stack.json examples/native-stack.json
# Conservative dependency blast-radius and overlapping resource estimate:
./gcontainers stack impact examples/native-stack.json examples/native-stack.json
# Pessimistic old+new overlap check, not a scheduler or deployment authorization:
./gcontainers stack transition-capacity examples/native-stack.json examples/native-stack.json --cpu-milli 4000 --memory-mib 2048 --max-instances 16
# Verify the caller-supplied old declaration fingerprint, not live runtime state:
source_sha="$(./gcontainers stack plan examples/native-stack.json | sed -nE 's/^[[:space:]]*"specSHA256": "(sha256:[0-9a-f]{64})".*/\1/p')"
./gcontainers stack revision-check examples/native-stack.json examples/native-stack.json --expected-current-sha256 "$source_sha"

# Read-only source revision, declared resource overlap and blast-radius checks together:
./gcontainers stack preflight examples/native-stack.json examples/native-stack.json --expected-current-sha256 "$source_sha" --cpu-milli 4000 --memory-mib 2048 --max-instances 16
# First-party inert Linux rootless namespace check (no OCI workload):
./gcontainers kernel-probe
./gcontainers validate /path/to/oci-bundle
# Inert guest layout check; nonzero exit for rejected policy or missing guest files.
# A successful preflight is NOT evidence that the guest can run or is secure.
./gcontainers preflight /path/to/oci-bundle
```

For experimental `oci create`, the preview checks that `rootfs/dev` and `rootfs/proc` are real directories (not missing or symlinked) **before** invoking `runc`/`crun` or writing the mutation journal. An optional, **experimental** rootless `/dev` profile permits only a private 64 MiB `tmpfs` with exact `nosuid,noexec,nodev,mode=755,size=65536k` options **paired** with a private `/dev/pts` `devpts` mount with exact `nosuid,noexec,newinstance,ptmxmode=0666,mode=0620` options. That profile additionally requires a real nonsymlink `rootfs/dev/pts` directory and continues to reject host bind mounts, custom devices and privileged syscalls. An exact mount allowlist is not proof of device-node or process isolation; **a first-party isolated real rootless OCI fixture passed; broader acceptance is incomplete**. Do not use this profile for production workloads.

Running an experimental rootless bundle also requires a maintained `crun` or `runc` installation, a private `XDG_RUNTIME_DIR`, a suitable Linux kernel, correctly mapped UID/GID ranges, rootfs, and an OCI spec satisfying the preview policy. Execution is not recommended on production hosts.

```sh
./gcontainers oci create --runtime runc --id example --bundle /path/to/oci-bundle --experimental
./gcontainers oci start --runtime runc --id example --experimental
./gcontainers oci state --runtime runc --id example
# Read-only pending-operation status (no patched OCI runtime required):
./gcontainers oci pending --id example
# Read-only private journal incident aggregate; no runtime required:
./gcontainers oci pending-summary
./gcontainers oci stop --runtime runc --id example --experimental
./gcontainers oci delete --runtime runc --id example --experimental
```

## Architecture, safety, and roadmap

- [Architecture](docs/architecture.md)
- [Security boundaries and limitations](docs/security.md)
- [Milestones and acceptance criteria](docs/ROADMAP.md)
- [Stable acceptance gate register — not yet passed](docs/STABILITY-GATES.md)
- [Platform deployment precedence](docs/DEPLOYMENT-PRIORITY.md)
- [Features and implementation evidence](docs/FEATURES.md)
- [Product objectives](docs/OBJECTIVES.md)
- [Docker and Podman migration strategy](docs/migration.md)

### Integration boundaries

- **GoreeCloud Mesh:** service discovery, policy-aware APIs, event delivery, and integration (future, not implemented).
- **GoreeCloud Identity:** authenticate and authorize remote management (future; no remote endpoint currently exists).
- **GoreeShield:** consent, data minimization, retention, and privacy controls (future adapter).
- **Wardveil:** container security policy, image scanning, detection and response (future adapter).
- **GoreeCloud UI:** the sole design authority for any future graphical management client.

## Current experimental capabilities

- [Private bundle snapshots](docs/PRIVATE-SNAPSHOT.md): bounded copy and digest, not immutable or authorized for execution.
- [Registry blob retrieval](docs/REGISTRY-PREVIEW.md): explicit HTTPS origin and exact descriptor, no authentication, redirects or full image resolution.
- [Pinned OCI manifest discovery](docs/MANIFEST-DISCOVERY.md): exact caller-supplied manifest digest and size, typed configuration/layer descriptors, no tag lookup or automatic downloads.
- [Pinned registry pull and inert staging](docs/STORE-ASSEMBLY.md): retrieve one explicit manifest and its referenced blobs, reuse verified CAS content and verify ordered uncompressed diff IDs. No executable image or tag resolution.
- [Content-store manifest assembly](docs/STORE-ASSEMBLY.md): verifies pinned manifest, config, compressed descriptors and ordered uncompressed diff IDs in inert staging.
- [Ordered inert layer assembly](docs/ASSEMBLY-PREVIEW.md): up to eight pinned layers; no whiteouts, links, executable permissions or full OCI semantics.
- [Private volume directories](docs/VOLUME-PREVIEW.md): exclusive creation, cooperative maximum 1,024 directories, aggregate inventory and bounded pinned offline backup/restore; no mounting, live application consistency, byte quota or deletion.
- [Local API](docs/LOCAL-API.md) and [client](docs/LOCAL-CLIENT.md): private same-UID Unix socket, read-only health, journal and opt-in redacted runtime counts, no TCP or MIPS identity grant.
- [Bounded lifecycle wait](docs/WAIT-PREVIEW.md): read-only stopped-state observation with pending-journal refusal; no exit code or instance identity guarantee.
- [Runtime inventory](docs/RUNTIME-INVENTORY.md): bounded private runtime metadata and pending-operation inventory, with per-ID observations and no repair.
- [Recovery observation](docs/RECOVERY-OBSERVATION.md), [start evidence](docs/START-EVIDENCE.md) and [stop evidence](docs/STOP-EVIDENCE.md): preserve uncertain journals and reject in-place marker changes with Linux change-time evidence; no automatic repair or forced escalation.

## Native replica orchestration planning

`gcontainers stack instance-plan SPEC [--expected-plan-sha256 DIGEST]` expands a validated declaration into deterministic per-replica IDs, complete dependency barriers, parallel startup waves and reverse shutdown waves. It checks a canonical plan pin on demand; reports remain dry-run only, with health and MIPS authorization false. See [instance planning](docs/INSTANCE-PLAN.md).

## Experimental inert layer staging

`gcontainers image stage-layer ABS_FILE tar|tar+gzip sha256:HEX DESCRIPTOR_BYTES PRIVATE_PARENT --experimental` stages a single digest- and size-pinned archive into a newly generated private directory under an existing user-owned mode-0700 parent. It uses the existing strict layer parser in the same hashed stream, rejects links, whiteouts, devices, privilege metadata and unsupported framing, and writes regular files mode 0600. Failed validation cleans only the new tree; unexpected identity changes require manual review. It is inert data staging, not a complete OCI rootfs unpacker, immutable snapshot, authenticated provenance or execution authorization. No existing tree is overwritten.

## Restricted experimental cgroup resource profile

Optional OCI Linux resources may declare all three bounded CPU quota/period (fixed 100000 microseconds), memory limit and PID limit values together. The real create path reads the actual process cgroup v2 membership and the exact cpu.max, memory.max and pids.max values before clearing the recovery journal. A mismatch fails closed and preserves recovery evidence. No host cgroup path, device rules or unbounded limits are accepted. On a nonproduction Linux laptop, `GC_SMOKE_RESOURCES=1 bash scripts/oci-smoke.sh --execute` verified a guest with 0.5 CPU, 64 MiB memory and 32 PIDs through creation, start, readback, stop and deletion. Resource exhaustion and hostile workload enforcement remain unverified.

## Isolated real-runtime acceptance fixture

On a disposable Linux x86-64 test environment, run `scripts/oci-smoke.sh --prepare-only` to build the first-party static guest and validate a fresh private bundle without launching it. The explicit `--execute` mode requires a root-owned upstream runc 1.5.3 matching the pinned official release digest. It tests create, running state after guest checks, SIGTERM, stopped state, delete and a clean journal. The fixture probes PID namespace, root mapping, no-new-privileges, seccomp, capability sets, read-only rootfs and denial of a socket syscall. The first successful isolated Linux x86-64 lifecycle was observed November 25, 2026 using upstream runc 1.5.3. The fixture's deny-default filter permits fstatfs and getdents64 for verified runtime bootstrap needs; it does not allow socket, mount, or namespace-changing syscalls. Host-side /proc status is checked against the actual guest process before stopping it. Never run this on production; failure retains private evidence and the guest self-expires. This is a narrow acceptance experiment, not Stable or proof of full confinement. The separate first-party `bash scripts/oci-concurrency-smoke.sh --execute` fixture also verifies two simultaneous rootless guest lifecycles under one private runtime root.

## Licensing

The development branch contains the existing [MIT License](LICENSE) notice. The protected source-admission candidate preserves both existing notices; source-rights verification remains open (issue #13). No third-party runtime source is vendored. OCI specifications and `crun`/`runc` are independent upstream projects with their own licenses.
