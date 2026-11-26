# Product roadmap and engineering gates

All items after M0 are **planned**, not implemented or scheduled promises.

| Milestone | Deliverables | Exit criteria |
| --- | --- | --- |
| M0 — Source foundation | Original Go CLI, bounded rootless OCI policy, crun/runc lifecycle adapter, tests, CI, docs, license | Build, vet, race tests, validation tests and code review |
| M1 — Real isolated runtime | Secure runtime-root handling, immutable bundle snapshots, reliable states and lifecycle, Linux rootless acceptance matrix | Real container start/stop/recovery tests; negative security tests; no known namespace/mount escape |
| M2 — Images and content store | OCI distribution/image, digests, verified layers, safe unpack, cleanup and image list/pull | Conformance fixtures, bad-archive and digest-tamper tests, storage quotas |
| M3 — Storage and networking | Named volumes, default-deny DNS/ports, rootless networks, resource controls | Fail-closed networking and storage ownership tests; data recovery |
| M4 — Application stacks | Native GoreeCloud stack validator/planner first; native executor, reconciliation, dependencies, healthchecks, rollback; optional Compose Spec importer and dry-run migration from Docker/Podman | Representative GoreeCloud Compose stack parity and rollback |
| M5 — Operations API | Rootless daemon, secure local API, identity and authorization, policy hooks, audit/metrics | Fuzz/security review, resource controls, least privilege, versioned contracts |
| M6 — Ecosystem integration | GoreeCloud Mesh, Identity, GoreeShield, Wardveil, GoreeCloud UI | Real integration, privacy/consent and security acceptance; accessible UI |
| M7 — VPS Mesh canary | Isolated staging stack only, parallel production, rollback and migration inventory | Same features/data/health/observability under load; recovery rehearsals |
| M8 — Verified stable default | Controlled cutover for each workload with tested rollback | No user-data loss, audited security, SLA evidence, operator acceptance |

Feature design order favors isolation and reversible migration over breadth. Container/build dependencies remain replaceable. The **stable outcome** is GoreeCloud Containers first, with optional second-priority Docker/Podman compatibility. Do not claim Docker/Podman have been replaced until M8 gates pass for each migrated workload. Cross-platform Compose compatibility is valuable but cannot gate all native workloads if the native format already provides complete independently tested capabilities.
