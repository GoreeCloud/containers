# GoreeCloud Containers — Changelogs

## 2026-10-18 — Bounded OCI capture and direct rootless CLI lifecycle

- PR #19 merged into Development `main` as `f8fa6ff32322820416f05cff07e29452ae4520b1`; Rust exact-head CI `39098626635` and post-merge CI `39099202111` succeeded.
- Replaced unbounded Unix capture-thread joins with bounded nonblocking stdout/stderr draining. OCI `create` error diagnostics use bounded Unix datagrams, protecting stopped init processes from an ordinary inherited pipe's SIGPIPE and preserving useful errors.
- Passed two explicit opt-in **direct `/usr/bin/runc`** lifecycle runs on the isolated, non-root Debian Development VPS, without the earlier test-only runtime wrapper. Both proved invalid-create diagnostics, `create → state → start → state → delete`, rootless identity, procfs, proof-of-work marker, and isolated cleanup.
- Still Development only. Complete OCI conformance, engine state persistence/recovery, networking, cgroup/resource isolation, Compose compatibility, all runtime failure cases and production Docker migration are **not accepted**.

## 2026-10-17 — Rootless procfs mount repair and isolated real-runc smoke

- PR #16 was squash-merged into `main` as `94ae8f8ca1dfd9de57ee0e95be6cfe3ef9152ca9`; exact-head Rust CI `39091756467` and post-merge Rust CI `39091834775` passed.
- The experimental rootless OCI profile now generates a narrowly typed `/proc` procfs mount and refuses missing or symlinked `rootfs/proc/` mountpoints before writing the OCI configuration.
- A gated, disposable real-`runc` smoke test ran as a non-root user with its own temporary runtime state, verified user-namespace UID/GID mapping and actual procfs filesystem type within the workload, and exited successfully. It is not executed by ordinary CI.
- This limited result does not establish complete GoreeCloud runtime lifecycle, OCI conformance, networking, storage/recovery, security/resource isolation, Compose compatibility, or production readiness; Docker remains in production.

## 2026-10-17 — OCI bundle publication input validation

- PR #13 merged as `8ded53d25e090b3c2cc9f69e005ef28409a20072`; OCI `root.path` must match the verified bundle `rootfs`, and mutable process fields are revalidated before creating `config.json`.
- Regression tests, exact-head CI and post-merge Rust CI succeeded.
- Rootless runtime execution, real workload lifecycle, OCI conformance, and Docker production migration remain unaccepted.

## 2026-10-17 — Experimental rootless OCI bundle profile accepted

- PR #11 was squash-merged into Development `main` at `94834eea6bf73aa6a1b6e772a9d1d9785327addc`.
- Added explicit OCI user-namespace configuration and single-ID UID/GID mappings for specified non-root host IDs, preserving `noNewPrivileges` and selecting a read-only root filesystem.
- Added `goree bundle init-rootless` configuration generation and deterministic mapping/host-root rejection tests; exact-head Rust CI passed before merge.
- The CLI's disposable bundle-generation test passed. This is **not** a real rootless workload or OCI runtime conformance acceptance.

## 2026-10-17 — Verified Development security stabilization

- PR #8 merged on `main` with Unix owner-only creation of OCI bundle configurations and a regression test.
- PR #9 merged on `main` with bounded runtime version probes and timeout/output-limit regression tests.
- Draft PR #4 remains unmerged; successful source tests do not establish real OCI workload or rootless acceptance.
- Docker remains the production runtime; Containers remains Development.

## 2026-09-27 — Project specification and project-record migration

- Added repository-native PROJECT-SPECIFICATIONS.md and PROJECT-RECORD.md.
- Reconciled historical GoreeCloud/goreecloud-containers naming to the live GoreeCloud/containers repository.
- Preserved accepted implementation evidence from PRs #1–#3 and accepted feature-state migration PR #5.
- Recorded draft PR #4 as unmerged Development candidate state rather than accepted implementation.
- Updated README and PLANNED-FEATURES.md for repository-native authority.
- Updated Platform Contract repository/evidence references without claiming a newer contract version or platform conformance.
- Retired competing root SPECIFICATIONS.md in the migration candidate.
- Kept both Drive project-specification sources protected pending review, merge, authoritative main readback, and final reconciliation.

## 2026-09-27 — Drive feature-roadmap migration

- Retired the synchronized Google Drive roadmap after Drive/repository parity verification.
- Moved feature-state authority to `IMPLEMENTED-FEATURES.md`, `PLANNED-FEATURES.md`, and this `CHANGELOGS.md`.
- Existing source-state and lifecycle boundaries remain unchanged; no lifecycle promotion is implied.
