# GoreeCloud Containers — Project Record

**Document Type:** Repository-Native Project Record
**Status:** Active
**Project:** GoreeCloud Containers
**Repository:** GoreeCloud/containers
**Authority:** Repository-local project record
**Last Updated:** 2026-10-18

## 2026-10-18 — Isolated timeout descendant-ownership gap reproduced

The accepted Development Unix `RuntimeExecutor` timeout path terminates and waits for the directly spawned runtime process, but does not own or reap arbitrary descendants. An isolated test on the non-root Debian VPS launched a disposable fake-runtime process that created a bounded short-lived descendant, emitted `timeout-diagnostic` to stderr, and then blocked. A 200-ms executor timeout returned `TimedOut` with the stderr diagnostic preserved; the independent descendant wrote a marker afterward and ended on its own. The focused test and full local offline Rust workspace tests passed. **This demonstrates that a direct-process timeout does not prove descendant cleanup**, and does not invalidate the previously accepted direct real-`runc` lifecycle smoke.

Blind process-group termination would risk killing an OCI container init deliberately left alive by a successful `create`. Remaining CONTAINERS-016 acceptance therefore requires runtime- and container-ID-scoped ownership tracking, bounded OCI state reconciliation, cleanup only when ownership is proven, safe retry/error reporting, and isolation tests against unrelated workloads. No Docker lifecycle action was performed. The test is not evidence of production readiness.

## 2026-10-18 — Bounded runtime capture accepted; direct no-wrapper OCI lifecycle verified

PR #19 was squash-merged into Development `main` as `f8fa6ff32322820416f05cff07e29452ae4520b1`. The Unix runtime executor now avoids unbounded output-reader joins when OCI descendants retain pipe descriptors. For OCI `create`, it captures limited stderr diagnostics through a nonblocking Unix datagram channel instead of discarding errors or leaving a SIGPIPE-prone ordinary pipe attached to a stopped container. Additional tests cover descendants retaining descriptors, post-create writes, error diagnostics, poll deadlines, oversized diagnostic datagrams, and capture truncation. Exact-head Rust CI `39098626635` and post-merge Rust CI `39099202111` passed formatting, Clippy, workspace tests and build. The accepted runtime source blob is `994fc3fd2a512dc118fabbd746b7d61bae6270a7`.

An isolated non-root Debian VPS Development checkout was synchronized to PR #19 head `63649e36a6598e4fe7d262a571547d0537b4f3d1` and the `goree` CLI built successfully using the offline local Rust toolchain. An explicitly opted-in disposable acceptance script then completed **two independent direct, no-wrapper** tests against real `/usr/bin/runc` (v1.3.6). Each test used a unique temporary `XDG_RUNTIME_DIR` and bundle, verified that a deliberately invalid rootfs generated promptly returned useful create-error diagnostics, then executed `goree bundle init-rootless` and `goree runtime create → state(created) → start → state → delete`. Its statically linked container probe confirmed mapped UID/GID 0:0, real procfs, and a writable test-only proof-of-work marker. Both test runs exited successfully; independent readback found no remaining temporary acceptance directories. Source worktree remained clean. No Docker container was modified, stopped or migrated.

**Acceptance scope:** This demonstrates the low-level GoreeCloud CLI rootless OCI lifecycle for a minimal disposable workload on one Debian host, and resolves the specifically reproduced inherited-output-pipe hang under those conditions. It does **not** establish broad OCI conformance, complete process-tree timeout cleanup for all runtimes, network/cgroup isolation, resource limits, persistent volumes/recovery, Compose interoperability, image-to-run workflows, or production Docker replacement. Image-to-bundle PR #4 remains Draft; the one-line Cargo metadata PR #14 was still separate and unmerged at this checkpoint. Production remains Docker.

## 2026-10-17 — Rootless procfs integrity and disposable runtime evidence

PR #16 was squash-merged as `94ae8f8ca1dfd9de57ee0e95be6cfe3ef9152ca9`. Prior to the change, a disposable real `runc` workload failed when the generated rootless OCI profile omitted a `/proc` mount: first `open /proc: no such file or directory`, then, with a plain directory added, `unsafe procfs detected: incorrect procfs root filesystem type`. A manual test-only procfs declaration fixed execution without changing any production container.

The accepted Development implementation provides a fixed `OciMount::Proc` declaration. Bundle initialization checks that `rootfs/proc/` exists as an actual directory, not a symlink, before writing owner-only `config.json`. Negative and positive regression tests pass; the ordinary minimal OCI configuration remains unchanged.

On the non-root Debian VPS, `scripts/real-rootless-runc-smoke.sh` was explicitly enabled and successfully executed the generated profile with a statically linked probe under `runc` using a unique temporary state directory. The probe verified container UID/GID 0 (mapped to the unprivileged host identity) and genuine procfs filesystem type, then exited successfully with automatic cleanup. The opt-in guard rejected an ordinary invocation. Local full Rust workspace tests and build passed; exact-head Rust CI `39091756467` and merged-main Rust CI `39091834775` passed formatting, lint, tests, and build.

This provides **one disposable real-runc rootless smoke result**, not acceptance of the native GoreeCloud high-level engine lifecycle, OCI conformance, all target environments, comprehensive isolation/resource controls, networking, storage, recovery, Compose, or production migration. Draft image-to-bundle PR #4 remains separate and unmerged; Docker production workloads were not altered.

## 2026-10-17 — OCI bundle input-validation hardening accepted

PR #13 was squash-merged into `main` as `8ded53d25e090b3c2cc9f69e005ef28409a20072`. Bundle initialization now refuses a mutable OCI `root.path` differing from the verified `rootfs` directory and revalidates process args, environment and working directory for malformed/NUL-bearing values before writing `config.json`. These controls complement, but do not establish acceptance of, the experimental rootless OCI profile from PR #11.

The integrated source passed local Rust workspace tests/build (including regression coverage); exact-head GitHub Rust CI run `39089690429` passed formatting, Clippy, tests and build. Post-merge `main` Rust CI `39089808179` succeeded. Accepted main OCI source blob `6b9511041f80cde169109057861677d6ce70441b` matched the locally verified source.

Cargo workspace repository-metadata correction PR #14 passed CI but remains **open and unmerged** because the authorized GitHub merge action was blocked. Draft image-to-bundle integration PR #4 and actual native rootless-runtime acceptance remain outstanding. Docker remains the production runtime; no Docker migration occurred.

## 2026-10-17 — Rootless OCI configuration foundation, not rootless runtime acceptance

PR #11, **experimental rootless OCI bundle configuration profile**, was merged as `94834eea6bf73aa6a1b6e772a9d1d9785327addc`. The exact-head Rust CI run `39088870983` passed formatting, Clippy, tests, and build. The Development CLI can generate an owner-only bundle configuration with explicit user namespace and non-root host UID/GID mapping, a read-only root filesystem, and `noNewPrivileges`. Unit tests and a disposable CLI configuration-generation smoke test passed.

The VPS's `runc spec --rootless` reference showed the need for explicit user namespace and UID/GID mappings; both the VPS and laptop permit unprivileged user namespaces. An isolated rootless Podman/`crun` Alpine smoke on the laptop passed, but this tests the **host**, not the GoreeCloud engine. No native real runtime lifecycle acceptance, conformance, resource isolation, or rootless production readiness is claimed.

PR #4 (verified-image-to-OCI-bundle integration) remains unmerged and Draft. Docker remains the production runtime; no container migration occurred.

The project-specification migration PR #7 was accepted on `main` as `b9131efd10abcbd10260db6885ece141fadffd52`, with passing post-merge Rust and Platform Contract checks and authoritative source readback. The historical Drive specification sources are retained pending final source-parity and migration-manifest reconciliation.

## 2026-10-17 — Accepted Development security milestones

PR #8 was merged on `main` as `46b89ee9fce2f69d1c401208f15f9a7aec51e250`. New OCI bundle configurations use Unix mode `0600` and the regression test passed.

PR #9 was merged as `6d49227d44beb05885d757641fe04a9e3880ffea`. Runtime version probes now use bounded execution, with timeout and output-limit regression tests. Both merges passed post-merge Rust CI.

Draft PR #4 remains unmerged, including additional Development-only bundle creation and runtime cleanup tests. Real workload, rootless, and production acceptance are not established. Docker remains production runtime.

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
