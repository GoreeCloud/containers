# GoreeCloud Containers — Project Record

**Document Type:** Repository-Native Project Record
**Status:** Active
**Project:** GoreeCloud Containers
**Repository:** GoreeCloud/containers
**Authority:** Repository-local project record
**Last Updated:** 2026-09-27

## 2026-09-27 — Project specification migration staged

GoreeCloud Containers project-specification authority is being migrated from transitional Google Drive sources into PROJECT-SPECIFICATIONS.md and PROJECT-RECORD.md.

The migration reconciles the historical repository name GoreeCloud/goreecloud-containers to the live GoreeCloud/containers repository.

Current GitHub main is authoritative for accepted implementation state. The Drive sources preserve broader product requirements and history and do not override newer repository evidence.

The two Drive project-specification sources were compared during migration. Their substantive requirement content is materially equivalent; the major line-level differences are formatting, especially bullet markers. The active DOCX is therefore the primary source while the legacy record remains represented through the preserved historical/product requirements.

Both Drive sources remain protected until this migration is independently reviewed, accepted, merged, read back from main, and reconciled without outstanding discrepancies.

## 2026-09-27 — Feature-state migration accepted

Pull request #5, "Migrate Containers feature tracking from Drive," was merged to main as 6c03948a70910472e8c9fbe8f1c7d61b1dc85cf1.

Its migration head was 0d458c22c8be74f5ae2d1ab42751548fff643441.

The migration established repository-native IMPLEMENTED-FEATURES.md, PLANNED-FEATURES.md, and CHANGELOGS.md.

Existing source-state and lifecycle boundaries remained unchanged.

## Current accepted main boundary

At the project-specification migration start, authoritative main is 6c03948a70910472e8c9fbe8f1c7d61b1dc85cf1.

Accepted source is Development and includes the verified image/content and low-level OCI/runtime foundations described in PROJECT-SPECIFICATIONS.md and IMPLEMENTED-FEATURES.md.

Key accepted boundaries include:

- no production replacement of Docker;
- no real external-registry acceptance;
- no accepted real crun/runc lifecycle behavior;
- no accepted full OCI conformance;
- no accepted rootless execution;
- no production networking/volumes/Compose/image-build stack;
- no accepted production platform-system integration;
- no Stable qualification.

## 2026-09-05 and later — Development OCI/image checkpoints

The Drive specification records the Development checkpoint that established the native Rust workspace, OCI image/content verification path, controlled OCI configuration and bundle foundation, runtime execution boundary, Development CLI, deterministic test fixtures, and explicit non-production acceptance limits.

These checkpoints are preserved as project history but are subordinate to the live accepted GitHub repository when a factual implementation-state conflict exists.

## Current draft PR #4 — image-to-bundle integration candidate

Draft pull request #4, "Integrate verified images into controlled OCI bundles," remains open and unmerged.

The live head observed during this migration audit is 2ae296d05fe9e7e344bdc49f07b3e3fdd79fa195.

The PR describes a previously validated exact head 542338f05b126e342b247a2ccf662b846dea1372 with Rust CI run 34001513355 passing formatting, Clippy with warnings denied, the full test suite, and build.

Candidate scope includes:

- a GoreeCloud-owned engine orchestration crate;
- supported image-process metadata mapping into OCI configuration;
- fail-closed handling for unsafe or ambiguous metadata;
- staged image/rootfs work;
- controlled publication of rootfs and config.json;
- deterministic image-to-bundle tests;
- Development CLI bundle-from-image flow;
- fake-runtime create/start/state/delete lifecycle handoff evidence.

This remains candidate Development evidence only.

It does not prove real crun/runc behavior, actual runtime state transitions, workload execution, kernel isolation, rootless correctness, networking, durable state, production deployment, Stable qualification, or Docker replacement.

## Enduring product decisions preserved from Drive

The migration preserves these project decisions:

- GoreeCloud Containers is a GoreeCloud-owned OCI-compatible engine/platform, not a Docker or Podman rebrand;
- crun is the preferred initial low-level runtime and runc an alternative;
- OCI image/runtime/distribution interoperability is core;
- rootless-first least privilege is a governing security objective;
- high-level engine semantics remain GoreeCloud-owned;
- image/content integrity and safe extraction are first-class;
- durable engine state must remain distinct from reconstructible caches;
- networking and storage require explicit identities and boundaries;
- Compose compatibility is a migration goal with explicit unsupported-feature behavior;
- image builds require reproducibility and supply-chain controls;
- remote APIs require authentication, authorization, audit, and versioning;
- Glaze UI applies to user-facing surfaces;
- Manager, Identity, Wardveil Security, Privacy Shield, Everkeep, Mesh, Metrics, and other platform integrations require evidence-backed acceptance;
- Docker production migration must be incremental, reversible, and evidence-backed;
- production and Stable claims require target-environment and recovery evidence.

## Authority transition

After this migration is accepted and verified on main:

- PROJECT-SPECIFICATIONS.md is the canonical project specification;
- PROJECT-RECORD.md is the canonical significant project history/evidence record;
- IMPLEMENTED-FEATURES.md and PLANNED-FEATURES.md own feature lifecycle state;
- CHANGELOGS.md owns release/repository change history;
- the old root SPECIFICATIONS.md is retired;
- Google Drive no longer remains a parallel project-specification authority.

The Drive source documents become permanently deletable only after authoritative main readback and final reconciliation succeed.
