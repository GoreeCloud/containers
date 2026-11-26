# Recovery observation preview

`gcontainers recovery-observe runc|crun ID` takes the cooperative per-ID lock (which may create a private lock file), reads a pending journal marker, queries the trusted runtime only when pending, and rechecks the marker. The report distinguishes an observed runtime state from an unavailable runtime response. It never clears a pending marker, retries a mutation, kills or deletes a workload, or authorizes recovery; a pending incident remains a failing exit code. No pending mutation is not evidence that a workload is healthy. Explicit authenticated reconciliation and durable recovery remain future work.

Recovery observation now rejects replacement of a pending marker even when its content is identical, including inode, mode, length, modification and Linux change-time checks around the runtime read. Synthetic marker replacement and rewrite regressions passed; no marker is cleared and no automated reconciliation is authorized. Same-UID races and runtime backend binding remain outside this evidence.

Runtime root and control-directory identity are checked before and after cooperative locks, and before runtime invocation and journal cleanup. Replacement of the named root or journal directory preserves the original pending marker and returns a recovery-required error. Tests replace the private directory during a synthetic callback; this does not provide atomic path binding or hostile same-UID isolation.

## Recovery assessment

gcontainers recovery-assess runc|crun ID [--expected-observation-sha256 DIGEST] derives a fixed review category from the existing anchored pending observation. For create/start/kill a matching state is a provisional observed postcondition only, never identity proof. Delete failure is never inferred as absence. The redacted assessment digest supports comparing a prior review with a fresh read; it is not a marker inode, PID or authorization pin. Unresolved operations return nonzero, journal remains unchanged, no runtime mutation or automatic clearing. Matrix tests and repeated marker preservation passed.

### Aggregate recovery assessment

The read-only recovery-summary runc|crun command aggregates at most 16 pending operations using per-ID anchored assessments and two matching inventory reads. Counts provisional postconditions, conflicting states, unavailable runtimes and delete reviews; no identifiers, process IDs, paths or observation digests are emitted. Any recovery incident returns nonzero and no marker is cleared. Non-atomic observation only; no authorization.
