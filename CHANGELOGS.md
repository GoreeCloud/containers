# GoreeCloud Containers — Changelogs

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
