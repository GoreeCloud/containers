# Migration from Docker and Podman (planning only)

Founder direction (November 15, 2026): after verified stable acceptance, GoreeCloud Containers is the **required default and first-priority deployment path** for GoreeCloud workloads. Docker, Docker Compose, and Podman are optional, second-priority compatibility/deployment alternatives and **must not be mandatory transitive runtime dependencies** of the stable engine. Older Docker Compose-first instructions are superseded for this target architecture. **Transitional operating rule:** existing functional Docker/Podman stacks remain in service without forced cutover until equivalent native capabilities, staging tests, data integrity, and rollback are verified. Migration is a controlled change, not an immediate removal of established engines.

1. Inventory *read-only* all workloads, OCI images, Compose files, bind mounts, named volumes, networks, secrets, ports, users, restart policies, and backup/restore ownership.
2. Define a per-workload compatibility matrix, including Compose features and unsupported flags; refuse unknown directives rather than silently ignoring them.
3. Implement interoperable OCI image import/export and Compose parsing. Do not copy credentials or secrets into the public repository.
4. Run production-like applications in isolated **VPS Mesh staging** with dedicated test data. Validate security, privacy, health, networking, throughput, resource use and incident recovery.
5. Rehearse real restore and rollback, then select individual low-risk canary workloads. Preserve the working incumbent and original data until accepted.
6. Promote to the default engine only for verified workloads. Record exact evidence, owner authorization for consequential cutovers, and remove obsolete configurations only after explicit decommissioning approval.

**Infrastructure boundary:** Primary Laptop for development and administration; VPS Mesh is the centralized hosting target. Retiring legacy servers remain migration-only, with no new workloads, agents or configurations. Do not deploy this prototype to legacy infrastructure.

**Current state:** no Docker/Podman workload migrated, no legacy engine removed, no production service changed.
