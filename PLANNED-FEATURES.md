# GoreeCloud Containers — Planned Features

> **Authority:** Repository-native planned-feature record. Plans are not implementation evidence.

**Status:** Active roadmap control
**As of:** 2026-09-27
**Authoritative project specification:** PROJECT-SPECIFICATIONS.md
**Canonical repository:** GoreeCloud/containers

## Purpose

This file records planned GoreeCloud Containers capability and lifecycle work.

It supplements PROJECT-SPECIFICATIONS.md and must not be used to promote unaccepted work.

Implementation state belongs in IMPLEMENTED-FEATURES.md. Significant project history belongs in PROJECT-RECORD.md. Release/change history belongs in CHANGELOGS.md.

## Roadmap

| ID | Feature / obligation | Priority | Current state |
| --- | --- | --- | --- |
| CONTAINERS-001 | Maintain canonical PROJECT-SPECIFICATIONS.md / PROJECT-RECORD.md and permanently retire Drive project-specification copies after accepted main readback. | High | Migration candidate |
| CONTAINERS-002 | Complete controlled image-to-bundle/runtime integration without promoting draft PR #4 until accepted. | High | Draft candidate |
| CONTAINERS-003 | Execute and accept representative real crun/runc lifecycle workloads and applicable OCI conformance evidence. | Critical | Planned / gated |
| CONTAINERS-004 | Validate real external-registry interoperability, reusable credential handling, image-index selection, and secure registry policy. | High | Planned |
| CONTAINERS-005 | Add signature, provenance, attestation, SBOM, and trust-policy capabilities where required. | High | Planned |
| CONTAINERS-006 | Prove rootless execution, user-namespace, cgroup/resource, security-module, and privilege boundaries. | Critical | Planned / gated |
| CONTAINERS-007 | Implement durable authoritative engine metadata with accepted backup, restore, export, and recovery behavior. | Critical | Planned |
| CONTAINERS-008 | Implement networking, port publication, service discovery, volumes, and persistence with rootless/security boundaries. | High | Planned |
| CONTAINERS-009 | Build higher-level engine workflows and a first-party versioned API; keep CLI as a client of the engine contract. | High | Planned |
| CONTAINERS-010 | Implement and validate Compose interoperability without silently ignoring material unsupported semantics. | High | Planned |
| CONTAINERS-011 | Add image-build workflows only after execution/lifecycle foundations are accepted. | Medium | Future |
| CONTAINERS-012 | Add applicable native/web management surfaces under the current Stable Glaze UI authority. | Medium | Future |
| CONTAINERS-013 | Integrate applicable Manager, Identity, Wardveil Security, Privacy Shield, Everkeep, Mesh, Metrics, and Glaze UI requirements. | High | Blocked / future |
| CONTAINERS-014 | Reconcile the repository Platform Contract declaration with current GoreeCloud platform governance. | High | Required before production qualification |
| CONTAINERS-015 | Preserve Docker as production runtime authority until migration pilots, rollback, recovery, compatibility, and production gates are separately accepted. | Critical | Governing boundary |

## Maintenance

Google Drive feature-roadmap synchronization is retired.

Update this file from verified repository implementation, current project specifications, applicable platform governance, and GoreeCloud Tasks Management. Missing obligations, stale status, duplicated work, or undocumented disposition changes are defects.
