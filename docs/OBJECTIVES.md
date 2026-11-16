# GoreeCloud Containers — Product objectives

**Mission:** Build a secure, self-hosted, maintainable GoreeCloud-owned Linux container platform that can permanently replace Docker and Podman as the GoreeCloud default **when stable**.

## Required outcomes

1. First-priority native GoreeCloud engine and CLI; a managed service can be added after secure local lifecycle acceptance.
2. No mandatory Docker, Docker Compose or Podman dependency in the stable runtime. Preserve optional, second-priority deployment and migration adapters.
3. Portable OCI artifacts and standards-based behavior with documented capability and conformance gaps.
4. Default-deny security: rootless execution, namespaces, user IDs, cgroups, LSM, seccomp, image provenance, constrained resources and auditable privilege boundaries.
5. Transactional upgrades and migrations with reliable volume/data preservation, rollback and rehearsal.
6. MIPS alignment with GoreeCloud Mesh (integration), GoreeCloud Identity (access), GoreeShield (privacy), Wardveil (cybersecurity) and GoreeCloud UI (design system).
7. VPS Mesh-only future server deployment and no new configurations on migration-only legacy infrastructure.
8. Honest per-feature status and reproducible CI plus isolated-host acceptance before a stable designation.

## Success criteria

A stable declaration requires observed runtime confinement, representative GoreeCloud workload parity, verified image and stack semantics, security and privacy acceptance, performance/resource tests, failure and recovery drills, clear operational runbooks, and measured canary outcomes. These gates have **not** been satisfied by the current 0.1 development preview.
