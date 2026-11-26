# Persistent local Stack preview — Beta candidate work

The explicit local preview persists an isolated restricted Stack across separate CLI invocations. It reuses the verified CAS, delegated static guest profile, pinned admission, bounded concurrent dependency waves, live kernel evidence and journaled engine. It is not production, Beta acceptance, general apply, authenticated MIPS ownership, networking, persistent volume attachment or restart-after-reboot.

## Commands

```sh
gcontainers stack start-local SPEC ADMISSION ADMISSION_SHA EXISTING_STORE PRIVATE_PARENT PRIVATE_LEASE_STORE --experimental
gcontainers stack status-local SESSION RECEIPT_SHA
gcontainers stack stop-local SESSION RECEIPT_SHA --experimental
gcontainers stack resume-stop SESSION RECEIPT_SHA --experimental
```

Use a dedicated existing owner-private mode-0700 parent for these sessions only. Retain the returned session directory and exact local receipt SHA-256 independently. Never substitute a pin computed from an unreviewed changed receipt. All restrictions of the finite exercise still apply. Start leaves running guests and durable private receipt/log. One active reservation per cooperating parent, with exclusive lock during controller operations; duplicate starts refuse. Use the same separate existing private mode-0700 lease store for every cooperating persistent session. The store serializes reservation changes and refuses any overlapping UID or GID across up to 64 durable records. Non-overlapping sessions can coexist. The operator must still prevent collisions with other stores, finite exercises and unrelated rootless tooling; this is not system-wide identity ownership. Do not use ordinary finite exercises or low-level commands against this session concurrently.

After live delegated checks, the start command records each random runtime ID, bundle path, metadata directory device/inode and boot digest, original PID and boot/PID/start-time process generation digest (private receipt only). Status checks those bindings, repeated inventory and exact pinned receipt. A naturally stopped process may be observed as stopped only when metadata and bundle binding match; no running identity claim for a stopped process. The report is not application health or MIPS authorization.

Stop requires all expected metadata and no pending operations before recording durable stop intent. Each signal/deletion checks the bound metadata and process generation under the engine's per-ID cooperative lock. Naturally stopped guests need no signal. Reverse waves stop/delete, verify empty inventory, write cleanup marker, then release the parent reservation. A new generation can start after verified cleanup; old receipt/slot cannot stop it. No force delete or unattended recovery/retry. Explicit resume-stop requires the exact prior durable stop intent and receipt pin, the same boot, unchanged bindings and no unresolved engine journal. A missing instance is acceptable only in an already reached reverse wave after independently verifying original process-generation absence. After successful cleanup, independently corroborate all original processes absent before releasing shared mapping and session reservations.

Failures retain the active reservation and session artifacts, including partial startup with no final receipt and interrupted stop. Keep the directory and consult recovery evidence. Do not remove the lease or replay a stop merely because a process is missing. Explicit resume-stop covers only interrupted shutdown with no unresolved engine marker and a complete final receipt. Startup interruption, reboot, unknown metadata, pending markers and partially committed release remain operator review cases. Same-UID filesystem tampering, process/metadata races outside cooperating tooling, power loss, and foreign rootless managers are not fully controlled. A start may leave running guests if the operator or command fails. The existing first-party acceptance guest self-expires; no supervisor restarts it. There is no application readiness probe or automatic boot service.

## Acceptance evidence

November 26, 2026, authorized Linux laptop with Go 1.27.4 and reviewed runc 1.5.3. Actual three delegated guests across two waves persisted after CLI exit; separate status observed all three; duplicate start, wrong receipt and obsolete stop refused; separate stop verified cleanup; second generation with fresh IDs/receipt reused the slot and completed cleanup. Shared-store cross-parent collision rejected before mutation; separate UID/GID collision unit tests and disjoint reservations passed. Three guests self-expired and were cleaned up without a signal. All three remained running and bound across six ten-second observation intervals (60-second synthetic soak). An isolated first-party interruption simulation persisted the exact stop intent, completed the first reverse wave separately and then resumed the remaining waves; ordinary stop refused the prior intent and resume without intent refused. The complete cleanup proved original process generation absence. Unit tests: persistent lock, cancellation, stale lock replacement, exact receipt/path/duplicate wave validation and process generation mismatch. Original finite exercise and repeated race tests, vet and build passed.

```sh
bash scripts/stack-exercise-smoke.sh --execute --persistent
bash scripts/stack-exercise-smoke.sh --execute --natural-exit
bash scripts/stack-exercise-smoke.sh --execute --persistent-soak
bash scripts/stack-exercise-smoke.sh --execute --interrupted-stop
```

## Beta acceptance checklist

- [x] Restricted pinned image and delegated guest execution.
- [x] Separate CLI start/status/stop and new generation in dedicated local slot.
- [x] Durable operation evidence, stale receipt refusal, no automatic destructive repair.
- [x] Synthetic 60-second three-guest soak, natural exit and explicit interrupted-stop recovery.
- [ ] Sustained representative application workload and crash-during-start or unresolved journal acceptance.
- [x] Shared durable identity reservations across cooperating persistent sessions using the same explicit lease store.
- [ ] Coordination with finite exercises, foreign runtimes and other stores.
- [ ] Actual isolated networking and durable attached storage with recovery tests.
- [ ] Workload readiness, controlled restart and recovery policy.
- [ ] Source rights, verified MIPS authorization, install/rollback and release acceptance for the intended Beta scope.

Stable also requires isolated VPS Mesh canary, production rollback and full acceptance. Beta is not accepted until outstanding gates are validated. Do not create a Beta release tag solely from this checklist or CI.

### Explicit prelaunch failure recovery

New persistent sessions write a private versioned prelaunch record before image preparation and a durable launch-intent marker before the first runtime mutation. Retain the prelaunchSHA256 from the start report independently. For a failed preparation with the record available:

```sh
gcontainers stack review-prelaunch SESSION PRELAUNCH_SHA
gcontainers stack cancel-prelaunch SESSION PRELAUNCH_SHA --experimental
```

Review requires the pinned record, matching session/runtime/mapping-store directory identities, an exact pre-mutation event log prefix, absent launch/stop/cleanup records, and two empty inventory observations under cooperating session and mapping-store locks. Cancellation writes terminal evidence before releasing matching unused reservations. It neither starts nor stops guests, clears journals, removes bundle contents or deletes failed-session records. A missing or malformed reference, earlier unsupported session, launch intent, pending operation, changed directory, or interrupted release requires operator review. This is a narrow prelaunch recovery path, not proof against external same-UID modifications, post-launch crash recovery or a power-loss guarantee. No manual marker removal.

November 26, 2026: actual pinned config mismatch, retained reservation, incorrect cancellation pin refused, read-only review, explicit cancellation, corrected fresh deployment of three nonroot guests, launched-session cancellation refused and verified cleanup passed in the isolated first-party fixture.

```sh
bash scripts/stack-exercise-smoke.sh --execute --prelaunch-failure
```
