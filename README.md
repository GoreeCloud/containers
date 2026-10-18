# GoreeCloud Containers

GoreeCloud Containers is GoreeCloud's first-party OCI-compatible container engine and workload-platform project.

## Current lifecycle

**Development** — version `0.1.0-dev.2`.

Docker remains the current GoreeCloud production container runtime until a separately validated migration changes that authoritative operational state.

The accepted source provides a controlled OCI runtime lifecycle foundation and a Development image/content pipeline. It does **not** establish full OCI conformance, accepted real crun/runc execution, rootless acceptance, durable engine state, production deployment, Stable qualification, or Docker replacement.

## Project authority

- [PROJECT-SPECIFICATIONS.md](./PROJECT-SPECIFICATIONS.md) — normative product, architecture, interoperability, security, platform, migration, and acceptance requirements.
- [PROJECT-RECORD.md](./PROJECT-RECORD.md) — significant project history, accepted milestones, draft-candidate boundaries, and evidence.
- [IMPLEMENTED-FEATURES.md](./IMPLEMENTED-FEATURES.md) — accepted implemented-feature state.
- [PLANNED-FEATURES.md](./PLANNED-FEATURES.md) — planned capability and obligation state.
- [CHANGELOGS.md](./CHANGELOGS.md) — repository-native release/change history.

After project-record migration acceptance and default-branch readback, GitHub is the authoritative location for project specifications and project history.

## Accepted Development foundation

Current accepted capabilities include:

- Rust 2024 workspace pinned to the repository toolchain;
- validated container identifiers and lifecycle transitions;
- typed minimal OCI Linux configuration generation;
- controlled bundle initialization;
- bounded explicit crun/runc process-execution primitives;
- SHA-256 image-content verification;
- bounded content-addressed storage;
- supported single-manifest OCI/Docker registry retrieval;
- image configuration and supported layer verification;
- diff-ID verification;
- restricted staged rootfs construction;
- Development image ingest/pull CLI paths;
- deterministic fixture-registry and fake-runtime tests;
- Rust CI and Platform Contract validation.

## Important acceptance boundaries

The current repository does not establish:

- reusable registry credential handling;
- multi-platform image-index selection;
- signature/provenance/SBOM trust policy;
- accepted external-registry interoperability;
- accepted real crun/runc lifecycle behavior or complete OCI conformance;
- accepted rootless execution;
- persistent authoritative engine metadata and recovery;
- production networking/volume management;
- Compose production compatibility;
- production deployment or Docker replacement.

Draft PR #4 contains later Development candidate work and remains unmerged.

## Development commands

```bash
cargo build --workspace
cargo test --workspace
cargo run -p goree -- version
cargo run -p goree -- container validate-id example-container
cargo run -p goree -- runtime probe crun
```

Development image and runtime commands are documented in the user manual and implementation documentation. They are not a stable production CLI contract.

## Additional documentation

- [User Manual](USER-MANUAL.md)
- [Features](FEATURES.md)
- [Benefits](BENEFITS.md)
- [Competitive Objectives](COMPETITIVE-OBJECTIVES.md)
- [Branding](BRANDING.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Dependencies](docs/DEPENDENCIES.md)
- [Security](docs/SECURITY.md)
- [Recovery](docs/RECOVERY.md)
- [Platform Conformance](docs/PLATFORM_CONFORMANCE.md)

## License

Original GoreeCloud Containers source is licensed under the Apache License 2.0. External runtimes and dependencies retain their own governing licenses and obligations.
