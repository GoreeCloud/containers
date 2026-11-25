# GoreeCloud Containers

**An experimental, GoreeCloud-owned Linux container engine and management platform.**

GoreeCloud Containers is being built as the eventual default containerization solution for the GoreeCloud ecosystem, with a design goal of permanently replacing Docker and Podman as the default management engines after independently verified stability. Docker, Docker Compose, and Podman remain optional, second-priority compatibility paths; none is required by the intended stable GoreeCloud Containers engine.

> **Status: 0.1.0-dev / experimental foundation. Not production-ready.** No Docker/Podman replacement, image pull/build, daemon, Compose execution, networking, volumes, UI, remote control, or cluster orchestration is claimed. Do not migrate production containers.

## Ecosystem deployment preference (founder direction, November 17, 2026)

For **eligible GoreeCloud applications, services, APIs and other deployable workloads**, prefer an **installed, independently validated GoreeCloud Containers** instance as the deployment target. GoreeCloud Containers is the *intended first-choice platform*, **not a currently proven production runtime**: no accepted production runtime or release is established.

Selection order, subject to the workload's verified compatibility and requirements:

1. **GoreeCloud Containers**, only when an appropriate version is installed, secure, supported, and independently accepted on the authorized target host.
2. **Docker and Docker Compose**, as supported, portable deployment alternatives.
3. **Podman and Podman Compose**, including rootless operation where suitable.
4. **Kubernetes**, where orchestration, scale, or multi-host requirements justify it.
5. **Other fit-for-purpose methods**, including systemd/native host services, VMs, and approved managed hosting for workloads that cannot or should not run in containers.

This preference does **not** require a rewrite of an otherwise portable OCI image or Compose deployment, the removal of existing Docker/Podman/Kubernetes support, or a speculative migration of a healthy live workload. Preserve application data, stateful recovery, networking controls, least privilege, secrets handling, signed/provenanced images, operational verification, and a tested rollback path. Record the reason when GoreeCloud Containers cannot be selected; retain a functioning supported fallback until native production acceptance is complete. Apply specific host placement and interim website-hosting policies separately.

This is a documented deployment **decision rule**, not evidence of an installation, benchmark, production acceptance, rollout, or completed integration. The canonical GoreeCloud governing instructions remain authoritative.

## Implemented in this development branch

- `gcontainers doctor`: a read-only report of local Linux and OCI runtime prerequisites.
- `gcontainers validate <bundle-directory>`: fail-closed parsing of a restricted rootless OCI bundle policy. Includes file-size and JSON nesting limits, duplicate-key rejection, and restrictions on host namespaces, root filesystem, capabilities, devices, hooks, writable/bind mounts, and seccomp defaults.
- `gcontainers preflight <bundle-directory>`: inert, read-only policy and guest-layout inspection with redacted failure reasons. Checks the executable, required guest directories, ELF interpreter and bounded nested shebang chain; rejects `/usr/bin/env` and `/bin/env` secondary command resolution. It also rejects default-deny seccomp declarations without an explicit guest `execve` allowance (a necessary but not sufficient bootstrap condition). It **never invokes an OCI runtime** and explicitly returns `isolationVerified=false` and `deploymentAuthorized=false` even on a positive result.
- `gcontainers oci create|start|state|stop|delete`: a GoreeCloud-owned lifecycle adapter that invokes an external, replaceable OCI low-level runtime (`crun` or `runc`) directly, without Docker, Podman, containerd, a shell, or a background daemon. Mutations require `--experimental`; runtime state is stored in a private per-user directory and deletion is never forced.
- OCI state reads strictly parse bounded runtime JSON and return only container ID, lifecycle state and PID. The reviewed `runc 1.5.3` response additionally includes string metadata `created`, `owner` and `rootfs`; these fields are recognized but **never displayed**, while unknown metadata, duplicate keys and mismatched IDs remain rejected. This compatibility allowance is not proof of successful OCI workload execution or release readiness.
- A journaled OCI `create` now performs an immediate strictly decoded `state` readback: a low-level runtime exit status 0 with a guest already `stopped` is **not** a successful container creation. In that case the pending journal is retained for operator recovery; passing this check does not guarantee the guest will remain alive or that subsequent `start` and confinement have been accepted.
- Additional preflight: the preview requires the exact invoking nonroot host UID/GID mappings and an approved root-owned system-installed low-level OCI runtime executable. No subordinate UID/GID ranges yet. Experimental execution now refuses external crun below 1.31 or runc below 1.5.3, and ambiguous versions; this is a conservative 2026-11-16 preflight, not a security certification.
- Read-only doctor reports crun_admission_status and runc_admission_status as not_installed, untrusted_executable, runtime_version_or_probe_rejected, or preview_preflight_passed. These are privacy-minimized preflight reasons, not deployment authorization; every real OCI operation still repeats independent runtime checks.
- Private local SHA-256 content-addressed blob store with bounded, directory-handle-anchored, atomic, deduplicated imports (requires explicit experimental confirmation; **no image unpack, registry client or container execution**).
- Read-only OCI image-layout integrity verification: referenced SHA-256 blobs, descriptor sizes, metadata limits, no downloads or extraction. Supported OCI media types are intentionally restricted; **this is not an image store or container image engine**.
- Native GoreeCloud stack JSON preview: strict, versioned schema validation and dependency-ordered, parallel-stage **dry-run plans only**, with a deterministic SHA-256 specification fingerprint (an audit identifier, not an approval). No Docker Compose engine is required; no stack apply or service deployment exists yet.

- Read-only inspection of a restricted tar/tar+gzip OCI layer subset: rejects path traversal, links, devices, whiteouts, extended metadata, duplicate paths, dangerous modes, oversized entries and decompression abuse. No extraction or filesystem writes.
- Further development safeguards: cooperating cross-process runtime locks and private incident-journal census; read-only full-store SHA-256 audit with advisory aggregate import quota; descriptor-anchored stack declarations and offline old-plus-new resource budget checks. None replaces isolation, signed provenance or production authorization.
- Standard-library-only Go code, initial unit tests and Linux CI.

The execution adapter is **not** yet independently security audited or exercised against real kernel isolation in this change. OCI runtimes remain required for execution; this is not a from-scratch kernel runtime. Validation reduces obvious hazards, but cannot certify an arbitrary OCI bundle as safe.

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

For experimental `oci create`, the preview checks that `rootfs/dev` and `rootfs/proc` are real directories (not missing or symlinked) **before** invoking `runc`/`crun` or writing the mutation journal. An optional, **experimental** rootless `/dev` profile permits only a private 64 MiB `tmpfs` with exact `nosuid,noexec,nodev,mode=755,size=65536k` options **paired** with a private `/dev/pts` `devpts` mount with exact `nosuid,noexec,newinstance,ptmxmode=0666,mode=0620` options. That profile additionally requires a real nonsymlink `rootfs/dev/pts` directory and continues to reject host bind mounts, custom devices and privileged syscalls. An exact mount allowlist is not proof of device-node or process isolation; **real rootless OCI execution using these mounts has not yet passed independent acceptance**. Do not use this profile for production workloads.

Running an experimental rootless bundle also requires a maintained `crun` or `runc` installation, a private `XDG_RUNTIME_DIR`, a suitable Linux kernel, correctly mapped UID/GID ranges, rootfs, and an OCI spec satisfying the preview policy. Execution is not recommended on production hosts.

```sh
./gcontainers oci create --id example --bundle /path/to/oci-bundle --experimental
./gcontainers oci start --id example --experimental
./gcontainers oci state --id example
# Read-only pending-operation status (no patched OCI runtime required):
./gcontainers oci pending --id example
# Read-only private journal incident aggregate; no runtime required:
./gcontainers oci pending-summary
./gcontainers oci stop --id example --experimental
./gcontainers oci delete --id example --experimental
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

## Licensing and provenance (source-admission review)

The documentation baseline is MIT-licensed, with the notice Copyright (c) 2026 GoreeCloud. The development branch carries an MIT proposal with the notice Copyright (c) 2026 GoreeCloud contributors. The candidate preserves both existing notices without asserting a new grant, changing existing rights, or declaring the engine provenance gate complete. The review must confirm copyright authority, origin of source and any third-party obligations before promotion. No third-party runtime source is vendored in the reviewed candidate; external OCI specifications, `crun` and `runc` remain independently licensed. See [source admission review](docs/SOURCE-ADMISSION.md).
