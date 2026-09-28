# GoreeCloud Containers — Project Record

**Document Type:** Repository-Native Project Record
**Status:** Active
**Project:** GoreeCloud Containers
**Repository:** GoreeCloud/containers
**Authority:** Repository-local project record
**Last Updated:** 2026-09-27

## 2026-09-27 — Repository-local project migration staged

GoreeCloud Containers project requirements and significant project history are being migrated from transitional Google Drive sources into PROJECT-SPECIFICATIONS.md and PROJECT-RECORD.md.

The migration reconciles the historical `GoreeCloud/goreecloud-containers` repository name to the live `GoreeCloud/containers` repository.

Current GitHub `main` controls accepted implementation truth. Drive remains a migration input for requirements and history until default-branch acceptance and readback.

The migration candidate retires root SPECIFICATIONS.md as a competing canonical specification and updates repository navigation and Platform Contract evidence references accordingly.

Both Drive project-specification sources remain protected until review/acceptance, merge, authoritative main readback, and final reconciliation succeed.

## 2026-09-27 — Feature-state migration accepted

Pull request #5, **Migrate Containers feature tracking from Drive**, was merged.

- PR head: `0d458c22c8be74f5ae2d1ab42751548fff643441`
- Merge commit: `6c03948a70910472e8c9fbe8f1c7d61b1dc85cf1`

The repository now maintains repository-native:

- IMPLEMENTED-FEATURES.md;
- PLANNED-FEATURES.md;
- CHANGELOGS.md.

No lifecycle promotion or production-runtime change was implied.

## 2026-09-07 — Draft image-to-bundle/runtime integration candidate

Pull request #4, **Integrate verified images into controlled OCI bundles**, remains open and draft.

Current exact head at migration time:

`2ae296d05fe9e7e344bdc49f07b3e3fdd79fa195`

Rust CI run `34160280632` passed on that candidate.

The candidate contains Development integration work and an opt-in real OCI lifecycle acceptance harness, but the successful ordinary CI run does not prove that a real crun/runc lifecycle workload executed.

PR #4 remains unmerged and must not be represented as accepted main implementation.

## 2026-09-05 — OCI image/content pipeline foundation accepted

Pull request #3, **Implement OCI image/content pipeline foundation**, was merged.

- PR head: `498fbc4d60c75dca1fb4a414e2084ccb34e52ef0`
- Merge commit: `d3cc21fbcd63767f6ebcbc3f3be6ee79b4db2370`

The accepted Development source added supported single-manifest OCI/Docker registry retrieval, SHA-256 content verification, bounded content-addressed storage, image-config/layer handling, diff-ID verification, restricted staged rootfs construction, and Development image-ingest/pull paths.

The acceptance did not establish reusable registry credentials, multi-platform image-index selection, signature/provenance trust, real external-registry acceptance, real OCI runtime acceptance, OCI conformance, rootless acceptance, durable state, production deployment, or Docker replacement.

## 2026-09-04 — Controlled OCI lifecycle execution foundation accepted

Pull request #2, **Implement controlled OCI lifecycle execution foundation**, was merged.

- PR head: `099c376c2d28fb71958d198b7432c0344e68aca9`
- Merge commit: `5f27f4237722ca6d9d3f3be473431f7da7bb4370`

This milestone established the bounded low-level runtime-execution foundation for explicit crun/runc operations and conservative executable/bundle handling.

Source-level/fake-runtime validation did not establish real runtime conformance or production acceptance.

## 2026-09-03 — Native Containers foundation accepted

Pull request #1, **Establish native GoreeCloud Containers development foundation**, was merged.

- PR head: `bbfa63561f36950e02dff02fc09a3d3d583ea5f4`
- Merge commit: `d87f94e1b3106d127de77067364a681a880ae025`

This milestone established the native Rust Development foundation, repository controls, lifecycle/state types, initial OCI configuration boundary, documentation, licensing, and CI.

The project was established as an original GoreeCloud container-engine architecture rather than a Docker/Podman fork.

## Product direction preserved from Drive

The Drive project specification established these enduring principles:

- GoreeCloud owns the high-level engine and management architecture.
- Mature OCI runtimes remain bounded low-level execution foundations.
- OCI interoperability is mandatory.
- Rootless operation is preferred where feasible.
- Docker remains production runtime authority until a separately validated migration.
- Image content uses integrity-verified content-addressed storage.
- Persistent workload data remains distinct from disposable writable layers.
- Compose interoperability is a strategic migration requirement.
- A first-party versioned API should become the canonical engine interface.
- GoreeCloud-controlled user interfaces must satisfy applicable Glaze UI requirements.
- Manager, Identity, Wardveil Security, Privacy Shield, Everkeep, Mesh, and Glaze UI responsibilities are separate acceptance requirements.
- Stable qualification requires real runtime, security, persistence, networking, recovery, compatibility, and migration evidence rather than source-level tests alone.

## Repository-authority transition

Historical documentation identified `GoreeCloud/goreecloud-containers` and a Drive project record as current authority.

The live repository is `GoreeCloud/containers`.

After the migration is accepted and verified on `main`:

- PROJECT-SPECIFICATIONS.md is the canonical project specification;
- PROJECT-RECORD.md is the significant historical/evidence record;
- IMPLEMENTED-FEATURES.md controls accepted implemented features;
- PLANNED-FEATURES.md controls planned feature state;
- CHANGELOGS.md controls repository-native change history;
- root SPECIFICATIONS.md is retired;
- Drive project-specification sources must be permanently removed.

## Current open governance boundary

The repository Platform Contract file currently uses schema 0.2 and contains historical repository and compatibility fields. Those values describe accepted repository state and do not prove compliance with the latest GoreeCloud platform contract or Glaze UI authority.

Platform Contract migration and accepted platform-system evidence remain separate obligations before production/Stable qualification.

The project remains Development.
