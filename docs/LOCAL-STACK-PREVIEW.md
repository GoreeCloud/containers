# Persistent local Stack preview — Beta candidate work

The explicit local preview persists an isolated restricted Stack across separate CLI invocations. It reuses the verified CAS, delegated static guest profile, pinned admission, bounded concurrent dependency waves, live kernel evidence and journaled engine. It is not production, Beta acceptance, general apply, authenticated MIPS ownership, networking, persistent volume attachment or restart-after-reboot.

## Commands

```sh
gcontainers stack start-local SPEC ADMISSION ADMISSION_SHA EXISTING_STORE PRIVATE_PARENT --experimental
gcontainers stack status-local SESSION RECEIPT_SHA
gcontainers stack stop-local SESSION RECEIPT_SHA --experimental
```

Use a dedicated existing owner-private mode-0700 parent for these sessions only. Retain the returned session directory and exact local receipt SHA-256 independently. Never substitute a pin computed from an unreviewed changed receipt. All restrictions of the finite exercise still apply. Start leaves running guests and durable private receipt/log. One active reservation per cooperating parent, with exclusive lock during controller operations; duplicate starts refuse. No global subordinate identity leasing: the operator must reserve disjoint allocated IDs across parents and all other tools. Do not use ordinary finite exercises or low-level commands against this session concurrently.

After live delegated checks, the start command records each random runtime ID, bundle path, metadata directory device/inode and boot/PID/start-time process generation digest. Status checks those bindings, repeated inventory and exact pinned receipt. A naturally stopped process may be observed as stopped only when metadata and bundle binding match; no running identity claim for a stopped process. The report is not application health or MIPS authorization.

Stop requires all expected metadata and no pending operations before recording durable stop intent. Each signal/deletion checks the bound metadata and process generation under the engine's per-ID cooperative lock. Naturally stopped guests need no signal. Reverse waves stop/delete, verify empty inventory, write cleanup marker, then release the parent reservation. A new generation can start after verified cleanup; old receipt/slot cannot stop it. No force delete or automatic recovery/retry.

Failures retain the active reservation and session artifacts, including partial startup with no final receipt and interrupted stop. Keep the directory and consult recovery evidence. Do not remove the lease or replay a stop merely because a process is missing. Same-UID filesystem tampering, process/metadata races outside cooperating tooling, power loss, and foreign rootless managers are not fully controlled. A start may leave running guests if the operator or command fails. The existing first-party acceptance guest self-expires; no supervisor restarts it. There is no application readiness probe or automatic boot service.

## Acceptance evidence

November 26, 2026, authorized Linux laptop with Go 1.27.4 and reviewed runc 1.5.3. Actual three delegated guests across two waves persisted after CLI exit; separate status observed all three; duplicate start, wrong receipt and obsolete stop refused; separate stop verified cleanup; second generation with fresh IDs/receipt reused the slot and completed cleanup. Unit tests: persistent lock, cancellation, stale lock replacement, exact receipt/path/duplicate wave validation and process generation mismatch. Original finite exercise and repeated race tests, vet and build passed.

```sh
bash scripts/stack-exercise-smoke.sh --execute --persistent
```

## Beta acceptance checklist

- [x] Restricted pinned image and delegated guest execution.
- [x] Separate CLI start/status/stop and new generation in dedicated local slot.
- [x] Durable operation evidence, stale receipt refusal, no automatic destructive repair.
- [ ] Sustained representative workload, natural exit and interruption/recovery acceptance.
- [ ] Shared identity leasing across all cooperating local deployment sessions.
- [ ] Actual isolated networking and durable attached storage with recovery tests.
- [ ] Workload readiness, controlled restart and recovery policy.
- [ ] Source rights, verified MIPS authorization, install/rollback and release acceptance for the intended Beta scope.

Stable also requires isolated VPS Mesh canary, production rollback and full acceptance. Do not create a Beta release tag solely from this checklist or CI.
