# Native Stack exercise — explicit local preview

A finite rootless runc controller for individually reviewed static ELF images and an independently pinned per-instance admission file. This is not general Stack apply, application readiness, authenticated MIPS admission or production deployment. It refuses secrets, limits the plan to 1–4 instances, 2 aggregate CPUs, 256 MiB aggregate memory, and requires each instance's explicit CPU/memory and 32 PID cgroup limits, exact restricted process policy and reviewed seccomp subset.

## Review and execute

Obtain the exact plan with the existing instance-plan command. Prepare a private (0400/0600) JSON admission with planSHA256 and instances, one entry per deterministic plan ID: id, manifestSize, absolute configFile, configSHA256, entrypointSHA256, whiteouts (boolean). The stack image digest must refer to the actual local CAS manifest. Each configuration must map guest UID/GID 1000 to distinct existing allocated subordinate IDs within this exercise. Independently review the source, executable and exact configuration/admission digests before execution.

```sh
gcontainers stack review-exercise SPEC ADMISSION ADMISSION_SHA
gcontainers stack exercise SPEC ADMISSION ADMISSION_SHA EXISTING_STORE PRIVATE_PARENT --experimental
```

Review checks the admission and plan pins, cardinality and resource budget; it does not verify image contents or authorize execution. Exercise prepares each pinned delegated bundle using existing CAS verification, checks resource/mapping/security compatibility, creates a fresh private runtime root and random session-specific runtime IDs (separate from deterministic plan IDs), and writes a private receipt and fsynced phase log before mutations. Instances in each wave create/start concurrently (maximum four); dependencies must be observed running before the next wave. Each running observation checks expected mapped identities, PID namespace evidence, no-new-privileges, seccomp, zero capabilities and cgroup limits. A running state is not a health check. Reverse dependency waves stop and delete using existing journaled engine; final empty inventory verifies metadata cleanup, not independent process-death attestation.

Any incomplete operation stops progression; already started siblings finish their bounded call. Preserve session, bundles, receipt and pending journal; no force cleanup, automatic rollback, restart or retry. Review the receipt and actual session runtime root before any manual recovery; never use IDs from an unrelated session. A guest may remain running after interruption. No network attachment, storage mount, credentials, unattended deployment or permanent service controller. The caller must independently ensure chosen subordinate identities are not in use by other workloads; the exercise enforces uniqueness only within its session. Same-user races, hostile actors and power-loss atomicity are not eliminated; session and journal evidence do not confer authorization.

## Acceptance

On the authorized laptop, the first-party synthetic CAS fixture started two independent nonroot replicas concurrently and one dependent replica in a second wave, checked live delegated kernel evidence, stopped/deleted in reverse waves and observed empty runtime metadata and journal. Stale admission pin rejected before allocating a session. Unit tests cover concurrent waves, interruption and no further mutation, duplicate IDs and randomized runtime identities. Go race tests, vet and build also exercised.

```sh
bash scripts/stack-exercise-smoke.sh --execute
```
