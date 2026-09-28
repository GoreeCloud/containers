# GoreeCloud Containers — Project Specifications

**Document Type:** Repository-Native Project Specification
**Status:** Active specification / Development
**Project:** GoreeCloud Containers
**Repository:** GoreeCloud/containers
**License:** Apache-2.0 for original GoreeCloud Containers source
**Authority:** Repository-local project specification
**Last Updated:** 2026-09-27

## 1. Purpose

GoreeCloud Containers is GoreeCloud's first-party OCI-compatible container engine and workload platform.

GoreeCloud owns the high-level engine contract, container lifecycle, image handling, registry interaction, networking orchestration, storage and volume management, policy, APIs, CLI behavior, user experience, state management, events, health integration, and GoreeCloud platform integrations.

Mature OCI standards, runtimes, kernel primitives, filesystems, networking primitives, security modules, cryptographic libraries, archive/compression libraries, and protocol libraries remain bounded technical foundations rather than product identity.

Docker and Podman interoperability are goals. Their private implementation architecture and product identity are not the permanent GoreeCloud architecture.

## 2. Current accepted implementation boundary

The project is in Development.

Authoritative main at the start of this migration is commit 6c03948a70910472e8c9fbe8f1c7d61b1dc85cf1.

Current accepted source includes:

- a Rust 2024 workspace pinned to Rust 1.85.0;
- validated container identifiers and guarded lifecycle transitions;
- deterministic in-memory Development state;
- typed minimal Linux OCI configuration generation;
- controlled no-overwrite bundle initialization;
- crun and runc runtime identities, probing, and low-level create/start/state/delete execution;
- direct runtime process spawning without a shell;
- bounded output and timeout handling;
- strict SHA-256 parsing and verification;
- bounded content-addressed image storage;
- supported OCI Image Manifest and Docker Registry v2 single-manifest retrieval;
- bounded anonymous Bearer-token handling;
- manifest/config/layer digest verification;
- Linux image config parsing and layer/diff-ID consistency checks;
- supported tar/gzip layer handling;
- uncompressed diff-ID verification;
- restricted layer extraction with path, symlink-parent, count, size, and unpacked-size protections;
- supported OCI whiteout handling;
- staged construction of a new rootfs;
- Development CLI paths for verified local content ingest and image pull/rootfs construction;
- deterministic fixture-registry, content-store, rootfs, fake-runtime, timeout, and failure-path tests;
- Rust CI and GoreeCloud Platform Contract declaration;
- repository-native feature-state records.

Current main does not establish:

- reusable registry credential authentication;
- multi-platform image-index selection;
- symbolic-link/hard-link layer extraction;
- signature, attestation, provenance, SBOM, or image trust-policy acceptance;
- real external-registry interoperability acceptance;
- real crun/runc lifecycle acceptance;
- full OCI conformance;
- accepted rootless execution or resource-boundary behavior;
- high-level image-to-container run acceptance;
- persistent engine metadata or recovery acceptance;
- production networking, volumes, Compose, image build, remote management, or Docker API compatibility;
- accepted GoreeCloud platform-system runtime integrations;
- production deployment;
- Stable qualification;
- Docker replacement.

Docker remains the current GoreeCloud production container runtime until a separately verified and accepted migration changes that operational state.

## 3. Candidate-state boundary

Draft pull request #4 is an unmerged Development candidate.

Its current source head at this migration audit is 2ae296d05fe9e7e344bdc49f07b3e3fdd79fa195.

The candidate develops controlled image-to-OCI-bundle construction, a GoreeCloud-owned engine orchestration crate, metadata mapping, staged bundle publication, Development CLI integration, and deterministic fake-runtime lifecycle handoff tests.

Its previously documented exact head 542338f05b126e342b247a2ccf662b846dea1372 passed Rust CI run 34001513355.

PR #4 remains candidate evidence only. It does not establish real crun/runc execution acceptance, actual container state transitions, kernel isolation, rootless correctness, networking, durable state, production deployment, Stable qualification, or Docker replacement.

## 4. Strategic architecture

The target architecture is:

GoreeCloud clients and management surfaces
→ GoreeCloud Containers API/CLI
→ GoreeCloud-owned engine
→ provider/runtime adapters
→ OCI-compatible low-level runtime and Linux platform primitives

The preferred initial low-level runtime is crun, with runc supported as an alternative.

Additional OCI-compatible runtime adapters such as youki, Kata Containers, or gVisor may be evaluated only when a demonstrated security, compatibility, isolation, or workload requirement justifies them.

GoreeCloud Containers must not initially reimplement Linux namespaces, cgroups, seccomp, OverlayFS primitives, SELinux/AppArmor enforcement engines, user-namespace primitives, or low-level OCI process supervision.

## 5. Repository and source-control model

The live canonical repository is GoreeCloud/containers.

Historical GoreeCloud/goreecloud-containers references are migration history only and must not be treated as the current repository identity.

The repository may organize engine, runtime adapters, image/registry handling, storage, networking, Compose compatibility, API contracts, clients, shared types, tests, platform adapters, and supporting documentation as the implementation evolves.

Separate native-client repositories should exist only when independent release lifecycle, platform build system, security boundary, or maintenance requirements justify separation.

## 6. OCI and ecosystem interoperability

OCI Image compatibility is a core requirement.

OCI Runtime compatibility is a core requirement for low-level runtime handoff.

OCI Distribution and standard registry interoperability are core goals.

Docker Registry compatibility should be preserved where practical.

Existing OCI images produced by Docker, Podman, Buildah, and compatible tooling should remain usable when they satisfy supported image/platform requirements.

Dockerfile compatibility may be supported where practical without making Docker implementation architecture the permanent GoreeCloud architecture.

Docker Compose compatibility is a major migration and interoperability objective, subject to explicit supported-feature classification and safe failure behavior.

Unsupported compatibility behavior must fail or warn clearly rather than being silently ignored when it could affect security, networking, persistence, availability, or workload semantics.

## 7. Rootless-first security model

The engine should request only privileges required by the workload and supported operation.

Administrative APIs and local control sockets must not become unrestricted host-root equivalents by default.

Runtime, networking, storage, device, secret, and administrative capabilities require explicit privilege boundaries.

Security-sensitive operations must fail closed when required authorization, identity, policy, or evidence is missing.

Rootless operation is a first-class target, but must not be described as accepted until representative user-namespace, cgroup, filesystem, networking, device, and runtime behavior has been verified.

## 8. Core engine responsibilities

The long-term engine responsibilities include:

- create, start, stop, restart, kill, pause where supported, inspect, execute within, log, and remove containers;
- pull, verify, store, inspect, tag, remove, export, and import images;
- create and manage networks, endpoints, port publication, DNS relationships, and supported service discovery;
- create and manage volumes, bind mounts, persistent storage relationships, and storage metadata;
- manage environment configuration, secrets references, health checks, restart behavior, resource limits, devices, and supported security profiles;
- expose versioned events, health, status, and administrative APIs;
- maintain durable engine metadata while keeping reconstructible runtime cache distinct from irreplaceable state.

No capability in this section is implemented merely because it is a requirement.

## 9. Image, content, and registry model

Image content must use content-addressed integrity where appropriate.

Expected registry/image requirements include:

- supported OCI/Docker manifest handling;
- digest verification before trust or publication;
- bounded downloads/responses;
- secure transport with explicit local-development exceptions;
- authenticated registry access through narrowly scoped credentials when implemented;
- supported multi-platform selection;
- image-config validation;
- layer/diff-ID verification;
- safe extraction;
- image trust/signature/provenance policy;
- SBOM/attestation support where accepted;
- safe garbage collection and retention.

Current Development content-store safety requirements include an absolute existing root, non-symlink endpoint, verified content before publication, and re-verification of existing blobs before reuse.

## 10. Engine state and data model

Stable identities should exist for images, containers, networks, volumes, runtime instances, stacks, secret references, health state, and relevant relationships.

A small transactional metadata store such as SQLite is an appropriate initial candidate if implementation evidence confirms it is sufficient.

Content-addressed image data should remain distinct from relational metadata.

Rootless and system-wide state locations must follow platform conventions and avoid unnecessary privilege.

Durable state requires documented backup, consistency, migration, and recovery semantics before production acceptance.

## 11. Networking

The product should support:

- isolated workload networks;
- controlled inter-network communication;
- explicit host port publication with collision/error behavior;
- container and service name resolution where appropriate;
- topology/identity reporting to approved GoreeCloud systems without unnecessary disclosure;
- policy boundaries for host, bridge, overlay, and future network modes where supported.

Networking requires representative target-environment validation before production claims.

## 12. Storage and volumes

Named volumes require stable GoreeCloud identities and inspectable ownership/attachment relationships.

Bind mounts require explicit host-path selection and clear security warnings when exposing sensitive host data.

Volume backup, restore, export, migration, and retention responsibilities must be explicit rather than implied.

Storage-driver choices should remain replaceable implementation details behind documented engine contracts where practical.

Persistent storage is not production-ready until restore and recovery evidence is available.

## 13. Compose and declarative workloads

Planned Compose-compatible functions include equivalents to:

- goree compose up;
- goree compose down;
- goree compose ps;
- goree compose logs;
- goree compose config.

Existing Compose files should be accepted only where their features map safely to supported GoreeCloud engine capabilities.

Unsupported semantics must be surfaced explicitly.

Declarative workload processing must preserve secrets, networking, storage, health, restart, dependency, and resource-limit boundaries.

## 14. Image builds

Image build support is a later capability and must not be confused with runtime image pull/verification.

Build functionality must address:

- Dockerfile-compatible behavior where useful;
- reproducibility;
- cache semantics;
- dependency/source provenance;
- secret handling;
- network policy;
- build isolation;
- SBOM/provenance output;
- signing/attestation integration;
- artifact verification.

Build workers must not receive unnecessary host or repository authority.

## 15. APIs and CLI

The CLI and API should expose the GoreeCloud product contract rather than raw provider/runtime internals.

The Development CLI may change before Stable.

Current Development commands include version, bundle initialization, local image ingest, image pull/rootfs construction, runtime probe/create/start/state/delete, and container-ID validation.

Planned higher-level interfaces include image, container, network, volume, Compose, status, events, logs, health, and administration workflows.

Remote APIs must be versioned, authenticated, authorized, auditable, and scoped before non-local use.

## 16. User interfaces and native clients

Linux should have a native management experience appropriate to local container execution and administration.

Windows and macOS may provide native management clients while using an explicitly managed Linux VM or equivalent platform virtualization for ordinary Linux container execution where required.

Android and iOS may provide native remote-management experiences where safe/useful; they are not expected to execute ordinary Linux containers directly.

A web management surface may be provided where it improves cross-device administration.

All implemented user-facing surfaces must use the applicable current Stable Glaze UI contract before Stable qualification.

## 17. GoreeCloud platform integration

Potential platform integrations include:

- GoreeCloud Manager for management/orchestration;
- GoreeCloud Identity for authenticated identity and claims;
- Wardveil Security for evidence-backed runtime, image, dependency, policy, and supply-chain security state;
- Privacy Shield for data minimization, retention, telemetry, and privacy policy;
- Everkeep for backup, recovery, portability, and preservation;
- GoreeCloud Mesh for coordinated service communication and events;
- GoreeCloud Metrics/Monitor for health and operational visibility;
- Glaze UI for user-facing interaction surfaces.

A declaration, stub, or schema-valid manifest is not runtime integration evidence.

Each integration requires explicit applicability, implementation, validation, failure behavior, and acceptance evidence.

## 18. Metrics and operational visibility

The platform should expose, where appropriate:

- engine health/readiness;
- container state, restarts, exit information, and health-check state;
- resource usage and configured limits;
- image, network, volume, runtime, and workload relationships;
- runtime adapter identity/version;
- event streams suitable for approved monitoring and coordination;
- bounded diagnostic information.

Observability must not unnecessarily expose secrets, environment values, private image data, or protected workload content.

## 19. External dependencies

crun is the preferred initial low-level OCI runtime candidate.

runc is an alternative runtime.

Kernel, filesystem, networking, cgroup, security-module, and virtualization facilities remain operating-system foundations rather than GoreeCloud reimplementations.

Every material dependency requires explicit versioning, vulnerability review, license review, provenance expectations, and an upgrade or replacement path.

Unsafe Rust is forbidden at the workspace lint level unless a later narrowly justified exception is explicitly governed and reviewed.

## 20. Production migration strategy

Current Docker production workloads continue operating while GoreeCloud Containers is developed.

Migration requirements include:

1. validate OCI compatibility, image behavior, rootless behavior, networking, storage, Compose semantics, security, backup, and recovery;
2. migrate low-risk test workloads first;
3. perform parallel validation against the current environment;
4. run selected production pilots only after controls and rollback are ready;
5. broaden migration incrementally and reversibly;
6. retire Docker as the primary runtime only after authoritative policies, standards, deployment records, and production evidence are deliberately updated.

Git or source-level acceptance alone must not cause runtime cutover.

## 21. Initial development sequence

### Phase 1 — OCI runtime and image foundation

OCI registry pull, image storage/unpacking, OCI runtime specification generation, crun/runc handoff, container lifecycle foundations, and deterministic local state.

Current main contains a subset of this phase. Real-runtime and target-environment acceptance remain open.

### Phase 2 — Rootless, networking, storage, resources

Rootless operation, networking, volumes, resource limits, health checks, restart behavior, and stronger security boundaries.

### Phase 3 — Compose compatibility

Representative Docker Compose migration and compatibility testing.

### Phase 4 — Image builds

Reproducible image builds and associated security/provenance controls.

### Phase 5 — Remote hosts and versioned API

Remote host enrollment, authentication/authorization, secure management, and versioned remote APIs.

### Phase 6 — GoreeCloud platform integrations

Substantive Manager, Identity, Wardveil Security, Privacy Shield, Everkeep, Mesh, Metrics, and Glaze UI integration.

### Phase 7 — Mature management clients

Native Windows/macOS/mobile clients where justified plus mature web administration.

## 22. Initial CLI experience

The target higher-level CLI should remain understandable and scriptable, including workflows analogous to:

- goree image pull alpine:3.23;
- goree container run --name test alpine:3.23 echo "Hello from GoreeCloud";
- goree container list;
- goree container inspect test;
- goree container logs test;
- goree container stop test;
- goree container remove test;
- goree compose up.

CLI ergonomics must not weaken authorization, safe defaults, or explicit failure behavior.

## 23. Initial non-goals

The initial engine must not:

- build a new kernel;
- reimplement Linux process-isolation primitives;
- build a new crun/runc equivalent before the higher-level engine is proven;
- attempt full Docker or Podman feature parity in the first release;
- require a proprietary hosted control plane or mandatory vendor account for core self-hosted operation;
- claim production replacement before migration and recovery evidence exists.

## 24. Production and Stable qualification

Production or Stable status requires evidence appropriate to the claimed scope, including:

- real external-registry interoperability;
- real crun/runc lifecycle acceptance;
- applicable OCI conformance;
- rootless behavior;
- networking;
- volumes and persistence;
- durable engine metadata;
- recovery/restore;
- Compose compatibility for advertised features;
- authentication and authorization;
- secrets protection;
- Wardveil Security and Privacy Shield acceptance;
- applicable platform-system integration;
- observability;
- upgrade and rollback;
- representative workload migration;
- sustained operation;
- incident/recovery behavior;
- current documentation and feature-state reconciliation.

Source CI and deterministic fake-runtime tests are necessary engineering evidence but are not production acceptance.

## 25. Governing principle

GoreeCloud Containers should own the product-level engine and user experience while relying on mature, replaceable open standards and low-level foundations.

The project should be interoperable without becoming architecturally subordinate to Docker, Podman, Forge-specific infrastructure, a proprietary cloud, or a mandatory vendor account.

## 26. Maintenance and authority

PROJECT-SPECIFICATIONS.md is the canonical project specification.

PROJECT-RECORD.md preserves significant project history and evidence.

IMPLEMENTED-FEATURES.md records verified implemented-feature state.

PLANNED-FEATURES.md records planned feature state.

CHANGELOGS.md records release/repository change history.

Google Drive project-specification copies are migration sources only. After this migration is accepted, verified on main, and reconciled, those Drive sources must be permanently deleted.
