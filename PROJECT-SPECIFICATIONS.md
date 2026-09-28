# GoreeCloud Containers — Project Specifications

**Document Type:** Repository-Native Project Specification
**Status:** Active specification / Development
**Project:** GoreeCloud Containers
**Backend Component:** GoreeCloud Container Engine
**Repository:** GoreeCloud/containers
**Authority:** Repository-local project specification
**License:** Apache-2.0 for original GoreeCloud source
**Primary Language:** Rust
**Current Version:** 0.1.0-dev.2
**Last Updated:** 2026-09-27

## 1. Product Purpose

GoreeCloud Containers is GoreeCloud's first-party OCI-compatible container engine and workload-management platform.

GoreeCloud owns the high-level product architecture, engine contract, lifecycle model, APIs, CLI behavior, image and registry handling, networking and storage orchestration, state model, policy, events, health integration, management experiences, and GoreeCloud platform integrations.

The product must remain interoperable with the broader OCI ecosystem and must not become a cosmetic wrapper around Docker, Podman, or another complete container-management product.

Docker remains the current GoreeCloud production container runtime and Compose-oriented deployment model until a separately validated migration changes that operational authority.

## 2. Current Accepted Implementation Boundary

The project is in **Development**.

Authoritative `main` at the start of this migration is commit:

`6c03948a70910472e8c9fbe8f1c7d61b1dc85cf1`

That main revision includes the accepted repository-native feature-state migration from pull request #5.

Accepted implementation milestones are:

- PR #1 — native GoreeCloud Containers development foundation;
- PR #2 — controlled OCI lifecycle execution foundation;
- PR #3 — OCI image/content pipeline foundation;
- PR #5 — repository-native feature-state migration.

The accepted source version is `0.1.0-dev.2`.

Draft pull request #4 remains an unmerged Development candidate. Its current exact head is `2ae296d05fe9e7e344bdc49f07b3e3fdd79fa195`, with Rust CI run `34160280632` passed. That candidate is not accepted implementation and must not be promoted by documentation migration.

## 3. Strategic Architecture

GoreeCloud Containers uses a middle-layer architecture:

- GoreeCloud owns the container engine and management platform.
- Mature OCI runtimes perform low-level container execution.
- `crun` is the preferred initial low-level runtime target.
- `runc` is an alternative runtime target.
- Additional OCI-compatible runtimes such as youki, Kata Containers, or gVisor may be evaluated only when a verified security, compatibility, isolation, or workload need justifies them.

GoreeCloud does not initially reimplement Linux namespaces, cgroups, seccomp, OverlayFS primitives, SELinux/AppArmor enforcement engines, user-namespace primitives, or low-level OCI process supervision.

A future first-party low-level GoreeCloud Runtime requires a concrete need that mature runtimes cannot satisfy safely and maintainably.

## 4. Product Identity and CLI

The user-facing name is **GoreeCloud Containers**.

The core backend component is **GoreeCloud Container Engine**.

Preferred command families include:

- `goree container`
- `goree image`
- `goree volume`
- `goree network`
- `goree compose`

A future local service may use a name such as `goree-containersd` only when a daemon architecture actually exists.

Names, service units, sockets, packages, and commands must track the implemented architecture rather than being documented as stable before implementation.

## 5. Repository and Licensing

The authoritative repository is `GoreeCloud/containers`.

Historical records may refer to `GoreeCloud/goreecloud-containers`; that name is historical and must not be used as current repository authority.

The repository should remain a coherent product repository unless independently released components acquire distinct lifecycle, security, packaging, or maintenance requirements that justify separation.

Original GoreeCloud source is licensed under Apache-2.0.

Third-party runtimes and dependencies retain their own licenses. Distribution must preserve applicable notices, attribution, source obligations, patent/license requirements, provenance, and dependency-license compatibility.

## 6. OCI and Ecosystem Interoperability

Open standards are interoperability foundations rather than proprietary boundaries.

Core requirements include:

- OCI Image compatibility;
- OCI Runtime compatibility for handoff to low-level runtimes;
- OCI Distribution / standard registry interoperability;
- Docker Registry compatibility where practical;
- use of supported OCI images produced by Docker, Podman, Buildah, and compatible tools;
- Dockerfile compatibility where practical;
- Docker Compose compatibility as a strategic migration goal.

Compatibility must be demonstrated with defined tests and representative workloads. File-format similarity alone is not acceptance evidence.

Unsupported behavior that could materially alter security, persistence, networking, availability, or workload semantics must fail or warn explicitly rather than being silently ignored.

## 7. Rootless-First Security Model

Rootless operation should be the normal model where workload requirements and operating-system capabilities permit it.

Rootful execution is an explicit elevation path.

Requirements include:

- minimum required privileges;
- no unrestricted root-equivalent control socket by default;
- explicit boundaries for runtime, networking, storage, devices, and secrets;
- fail-closed behavior when authorization or required security evidence is unavailable;
- documented cases where rootless mode cannot satisfy a workload requirement;
- visible and deliberate elevation when privileged execution is required.

Rootless support is not accepted until target-environment evidence proves the claimed execution and isolation behavior.

## 8. Current Workspace and Accepted Development Capabilities

The current Rust workspace includes:

- `goreecloud-containers-core` — identifiers, lifecycle states, transitions, and Development state;
- `goreecloud-containers-image` — digest verification, bounded content-addressed storage, registry retrieval, image metadata, and controlled rootfs construction;
- `goreecloud-containers-oci` — typed minimal OCI Linux configuration and bundle initialization;
- `goreecloud-containers-runtime` — crun/runc selection, probing, planning, validation, and controlled process execution;
- `goree` — Development CLI.

Unsafe Rust is prohibited at the workspace lint level.

Accepted Development capabilities include:

- validated container identifiers and lifecycle transitions;
- Development in-memory state;
- typed OCI configuration generation;
- controlled no-overwrite bundle initialization;
- low-level runtime probing and create/start/state/delete execution;
- bounded direct child-process execution without a shell;
- strict SHA-256 parsing and content verification;
- bounded content-addressed image storage;
- supported single-manifest OCI/Docker Registry v2 retrieval;
- image-config and layer retrieval;
- compressed-content and uncompressed diff-ID verification;
- restricted tar/gzip layer extraction;
- supported OCI whiteout handling;
- staged construction of a new rootfs target;
- Development image ingest/pull CLI paths;
- deterministic fixture-registry and fake-runtime test coverage.

These are Development capabilities, not proof of production OCI compatibility or Docker replacement.

## 9. Image, Registry, and Content Model

Image content must use content-addressed storage and integrity verification appropriate to OCI artifacts.

The engine must distinguish among:

- immutable content;
- image metadata;
- writable container state;
- configuration;
- secrets references;
- durable engine metadata;
- runtime-temporary state.

Registry communication must be standards-based, authenticated where required, and independent of a mandatory proprietary hosted control plane.

Current accepted source supports bounded anonymous/Bearer pull authorization for the implemented Development path. Reusable registry credentials are not accepted.

Future requirements include:

- reusable credential handling with secure storage and least privilege;
- multi-platform image-index selection;
- signature/provenance/attestation policy;
- SBOM handling where applicable;
- trust-source separation;
- external-registry interoperability evidence;
- bounded downloads and decompression;
- secure redirect/transport policy.

Public or non-loopback registry transport must use secure transport unless an explicitly bounded Development fixture requires loopback HTTP.

## 10. OCI Bundle and Runtime Execution

Generated OCI configuration must remain explicit and typed.

Current Development configuration targets OCI Runtime Specification 1.3.0 and applies conservative defaults including `noNewPrivileges`.

Bundle creation must:

- require an absolute existing bundle path;
- reject unsafe symlink endpoints;
- require a valid rootfs;
- refuse unsafe overwrite of configuration;
- use controlled file-creation semantics.

Runtime execution must:

- explicitly select the runtime kind;
- validate and canonicalize the executable path;
- avoid shell command construction;
- bound stdout/stderr retention while continuing to drain output;
- enforce reasonable timeout behavior;
- preserve non-zero exit status and bounded diagnostics;
- validate bundle/config inputs.

Fake-runtime test success is source-boundary evidence only. Real crun/runc acceptance requires actual disposable target execution and separate conformance evidence.

## 11. Engine Lifecycle Responsibilities

The future GoreeCloud Container Engine should own high-level lifecycle behavior for:

- create;
- start;
- stop;
- restart;
- kill;
- pause where supported;
- inspect;
- exec;
- logs;
- remove.

It should also own the lifecycle of images, networks, volumes, stacks, configuration, health checks, restart policy, devices, resource limits, and supported security profiles.

High-level engine behavior must remain separate from the low-level runtime adapter boundary.

## 12. Engine State and Data Model

GoreeCloud Containers must maintain a GoreeCloud-owned state model rather than using Docker internals as its product contract.

Stable identities are required for applicable:

- images;
- containers;
- networks;
- volumes;
- runtime instances;
- stacks;
- secret references;
- health state;
- relationships.

A transactional metadata store such as SQLite may be suitable initially if implementation evidence confirms it meets requirements.

Content-addressed image data must remain separate from relational metadata.

Reconstructible cache/content must remain distinguishable from irreplaceable durable state for backup, recovery, cleanup, and migration.

No unimplemented on-disk schema may be described as stable.

## 13. Networking

The engine layer should own workload-network intent while relying on operating-system networking facilities and bounded mature helpers where appropriate.

Requirements include:

- isolated workload networks;
- controlled inter-network communication;
- explicit port publication;
- clear collision/error behavior;
- supported container/service name resolution;
- rootless-compatible ordinary networking where feasible;
- no unnecessary broad host-network privilege;
- controlled topology exposure to approved GoreeCloud platform systems.

Networking acceptance requires real target-environment evidence.

## 14. Storage and Volumes

Persistent workload data must remain separate from disposable writable layers.

Requirements include:

- stable identities for named volumes;
- inspectable ownership/attachment relationships;
- explicit host-path selection for bind mounts;
- warnings for sensitive host-path exposure;
- clear backup, restore, migration, export, and retention responsibilities;
- replaceable storage-driver implementation behind documented contracts where practical.

Volume backup creation alone is not recovery acceptance; representative restoration and relationship validation are required.

## 15. Compose and Declarative Workloads

Compose compatibility is a strategic requirement because GoreeCloud currently relies on Compose-style deployment and because interoperability reduces migration risk.

Planned functions include equivalents of:

- `goree compose up`;
- `down`;
- `ps`;
- `logs`;
- `config`.

Existing Compose files should be supported where their features map safely to accepted GoreeCloud engine capabilities.

GoreeCloud-specific optional extensions may use an `x-goreecloud` namespace where useful, provided ordinary compatible tools can safely ignore them.

## 16. Image Builds

Image building follows stable execution/lifecycle foundations rather than being part of the minimum engine milestone.

A mature open-source build foundation may initially sit behind a GoreeCloud-owned build contract.

Future build requirements include:

- reproducible contexts;
- controlled secrets;
- cache management;
- provenance;
- multi-stage builds;
- multi-platform workflows where justified;
- Dockerfile interoperability where practical.

A native build subsystem requires a concrete capability, security, reproducibility, performance, or integration justification.

## 17. Native API and CLI

GoreeCloud Containers should expose a first-party versioned engine API as the canonical client/integration contract.

The CLI should be a native client of that API rather than a separate implementation of engine logic.

A Docker-compatible API may later exist as an interoperability layer, but compatibility endpoints must not dictate the internal GoreeCloud architecture.

Current Development CLI paths are not a stable public contract and may change until separately accepted.

## 18. User Interfaces and Native Clients

Planned management surfaces include:

- Linux native management;
- Windows and macOS native management clients;
- Android/iOS remote-management clients where safe and useful;
- a web management application where useful.

Windows/macOS local Linux-container execution may use explicitly managed Linux virtualization where required.

Mobile applications are management clients, not ordinary local Linux-container runtimes.

All GoreeCloud-controlled user interfaces must satisfy the current applicable Stable Glaze UI authority before Stable qualification.

Native clients should match host-platform expectations rather than defaulting to thin wrappers without a documented reason.

## 19. Platform-System Integration

GoreeCloud Containers must be evaluated against all current required GoreeCloud Platform Systems.

### GoreeCloud Manager

Manager may provide authorized visibility and lifecycle controls without bypassing Container Engine safety or Identity authorization.

### GoreeCloud Identity

Identity governs applicable human, service, device, application, and workload authority.

### Wardveil Security

Wardveil requirements include trust, policy, workload hardening, image/supply-chain verification, privileged-operation protection, event handling, and administrative safeguards.

### Privacy Shield

Privacy Shield governs telemetry, logs, diagnostic collection, metadata visibility, retention, remote-management flows, and exposure of sensitive workload configuration.

### Everkeep

Everkeep governs backup, restore, portability, migration, preservation, recovery verification, and succession requirements for important engine state and workload data.

### GoreeCloud Mesh

Mesh may provide capability discovery, events, relationships, dependencies, and coordination without becoming an authorization bypass.

### Glaze UI

Glaze UI governs applicable first-party UI accessibility, interaction, responsive behavior, status presentation, and safe representation of security/privacy/identity/health/recovery state.

The repository's current Platform Contract file uses schema 0.2 and historical repository/version fields. That is accepted repository state, not proof that schema 0.2 or its recorded Glaze UI requirement remains current platform authority. It must be reconciled before production/Stable qualification.

## 20. Metrics and Operational Visibility

The engine should expose bounded operational state sufficient for:

- health and readiness;
- container state, restart, exit, and health information;
- resource usage and configured limits where available;
- image/network/volume/runtime relationships;
- runtime adapter identity/version;
- events for approved monitoring and coordination.

GoreeCloud Metrics remains authoritative for broader infrastructure telemetry and historical analysis.

## 21. External Dependencies

Material dependencies must have:

- explicit versioning;
- provenance;
- vulnerability review;
- license review;
- upgrade/replace path;
- bounded purpose.

Current accepted source uses mature Rust dependencies for hashing, HTTP/URL handling, serialization, decompression, and archive parsing.

crun/runc remain external runtime foundations rather than GoreeCloud-owned components.

GoreeCloud must not silently absorb a complete third-party container manager as its permanent architecture.

## 22. Production Migration Strategy

GoreeCloud Containers is not an immediate production replacement for Docker.

Migration requirements include:

- preserve existing Docker workloads until a separately validated cutover;
- prove OCI behavior;
- prove rootless/security behavior;
- validate image compatibility;
- validate networking and persistence;
- validate Compose semantics;
- validate backup/recovery;
- validate monitoring/operations;
- maintain rollback capability;
- stage migration pilots before production authority changes.

A source-level green CI run or successful fixture test is not sufficient to change production runtime authority.

## 23. Initial Development Sequence

The intended engineering progression is:

1. core types, lifecycle model, repository controls, and OCI configuration;
2. controlled low-level runtime execution;
3. image/content and registry pipeline;
4. image-to-bundle/runtime integration;
5. real runtime acceptance;
6. rootless and security/resource boundaries;
7. durable state and recovery;
8. networking and storage;
9. higher-level engine workflows and API;
10. Compose/interoperability;
11. management surfaces and platform integrations;
12. production migration evidence.

This sequence may evolve, but safety and acceptance gates must not be skipped merely to accelerate feature breadth.

## 24. Explicit Initial Non-Goals

The initial engine does not require:

- a GoreeCloud reimplementation of the Linux kernel container primitives;
- immediate Docker replacement;
- complete Compose compatibility;
- all registry authentication models;
- all platforms;
- full graphical administration;
- image building;
- every OCI artifact type;
- every runtime adapter.

Unsupported areas must remain explicit rather than being implied as implemented.

## 25. Production and Stable Qualification

Stable or production qualification requires evidence appropriate to the claimed scope, including as applicable:

- real crun/runc lifecycle acceptance;
- OCI conformance;
- representative external registry interoperability;
- rootless/isolation/security acceptance;
- persistent state and recovery;
- networking;
- volumes;
- health/restart/resource controls;
- API/CLI contract stability;
- Compose compatibility;
- monitoring and diagnostics;
- platform-system integration;
- upgrade and rollback;
- representative migration pilots;
- supported-platform validation.

Docker remains production authority until those migration gates are satisfied and a separately accepted transition changes that state.

## 26. Maintenance

PROJECT-SPECIFICATIONS.md must evolve with material scope, architecture, runtime, image/registry, state, network/storage, compatibility, security, privacy, platform integration, deployment, or acceptance changes.

PROJECT-RECORD.md preserves significant historical developments and evidence.

IMPLEMENTED-FEATURES.md controls accepted implemented-feature state.

PLANNED-FEATURES.md controls planned feature/obligation state.

CHANGELOGS.md is the repository-native release/change-history authority. Legacy CHANGELOG.md may remain supplementary only if it has a distinct justified purpose and must not replace CHANGELOGS.md.

After project-record migration is accepted and read back from the default branch, Google Drive must no longer remain a parallel project-specification or project-record authority.
