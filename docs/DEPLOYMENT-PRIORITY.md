# GoreeCloud Containers — Deployment platform priority

Founder directive, November 15, 2026. Status: **target policy**, not proof of a stable release.

## Permanent priority after stable acceptance

1. **GoreeCloud Containers** — required first-priority engine and management path for applicable self-hosted GoreeCloud workloads, natively supported and independently operable.
2. **Docker, Docker Compose, and Podman** — optional second-priority compatibility and alternative deployment paths, supported where feasible but never mandatory to operate the stable GoreeCloud engine.

OCI image and runtime standards remain interoperability contracts; implementing them does not require Docker or Podman. Minimize external dependencies without replacing proven Linux kernel security primitives with unaudited home-grown isolation.

## Transitional precedence

Current operational Docker Compose stacks are **not** automatically migrated or forbidden while native engine functionality is incomplete. Existing deployments must remain recoverable and supported until verified per-service parity, isolated staging, negative security tests, backup/recovery/rollback gates, documented sign-off and production cutover. Do not move workloads to the prototype or install new services on retiring legacy hosts. VPS Mesh is the sole approved forward-looking VPS target.

## Acceptance

GoreeCloud Containers can only become the operational default after it independently demonstrates image management, rootless isolation, reliable lifecycle, secure networking, persistent volumes, stack deployment, observability, recovery, MIPS boundaries, and workload-specific canary/rollback acceptance. A CI pass or a CLI prototype is not evidence of this milestone.

## Responsibility

This repository records implementation status. The canonical GoreeCloud Google Drive governance still contains former Docker Compose-first wording and must be synchronized in place under verified authority. This page documents the founder's newer direction but is not a substitute for that Drive update.
