# Architecture — planned and current

## Objective

Deliver a GoreeCloud-owned Linux container platform that directly manages OCI containers, image lifecycle, networking, persistent storage, application stacks, and optional multi-host orchestration; provide migration from Docker/Podman without forcing vendor-specific app changes.

## Current prototype (0.1-dev)

```text
gcontainers CLI (Go)
   | doctor / validate
   | OCI lifecycle actions (explicit --experimental)
   v
internal/bundle: bounded, strict OCI JSON preview policy
internal/engine: state-root enforcement, ID validation, exec-without-shell
   |
   +--- crun OR runc (external OCI runtime, replaceable)
         |
         +--- Linux namespaces, cgroups, seccomp, LSM and kernel
```

There is no daemon, socket API, registry client, local image store, networking controller, volume manager, builder, service scheduler, or web interface at this stage. The external low-level OCI runtime executes containers; the GoreeCloud code owns the initial lifecycle boundary and policy checker.

## Intended modules (not implemented)

| Module | Proposed boundary |
| --- | --- |
| `gcontainersd` | Rootless daemon, versioned local Unix socket, peer credentials, authorization and audit events |
| Image service | OCI image/distribution protocols, content-addressed blob storage, digest/signature checks, secure unpack and garbage collection |
| Runtime | OCI lifecycle, crash recovery, rootless user namespaces, cgroup v2, seccomp and LSM |
| Network | Default-deny bridge and rootless networking, explicit published ports, DNS integration |
| Storage | Named volumes, mount ownership, snapshot interfaces, recovery rehearsals |
| App stacks | Compose Spec importer with explicit compatibility diagnostics, transactional apply and rollback |
| Scheduling | Health probes, restart policies, resource quotas, upgrades and controlled multi-host placement |
| API and UI | GoreeCloud Identity + Mesh + GoreeShield + Wardveil; GoreeCloud UI for clients |

## Engine priority and self-sufficiency

Stable target: the GoreeCloud-owned engine, CLI, configuration, image and stack management are the primary deployment path. Docker/Podman engines are **optional, second-priority alternatives**, not required components. OCI interoperability is preserved as a standard, not as a mandate to install another management engine. The 0.1 preview's optional low-level OCI runtime (crun/runc) **does not yet satisfy independent native-runtime acceptance**. Future replacement or permanent isolated dependency selection is subject to M1 security review.

## Dependency strategy

Use Linux kernel isolation rather than inventing a new kernel. OCI specs preserve workload portability. Maintain an abstraction for `crun` and `runc` initially; replacement or in-house low-level runtime work requires an independently audited security case and a sustained maintenance budget. No dependency on Docker or Podman is required by the initial adapter.

## Non-goals until assessed

No Kubernetes distribution, no rootful-by-default daemon, no Docker socket impersonation, no implicit network exposure, no privileged host bind mounts, no production cutover, and no data migration without backups, restore checks and rollback.

## Target operating environment

Primary development is the founder's Linux laptop; VPS Mesh is the sole designated VPS for future staging and deployment. Retiring legacy infrastructure remains migration-only, with no new workloads or configurations. Production remains unchanged in this milestone.
