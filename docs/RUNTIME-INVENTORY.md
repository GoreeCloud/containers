# Read-only private runtime inventory

gcontainers inventory runc|crun scans the existing private runtime root and journal, excludes reserved lock/journal directories, bounds the combined inventory to 128 IDs, rejects unknown/symlink/unsafe entries, and reports minimal per-ID runtime state with pending operation indicators. Reads use existing cooperative locks; a lock file may be created. A second anchored scan rejects changed directory or marker identity, including Linux change time. Uncertain state or pending operations return a report and nonzero status; no marker is cleared. No bundle paths, PIDs, annotations or raw runtime output are returned. Runtime backend binding and atomic point-in-time observation are not established. Synthetic inventory, missing state, pending-only and symlink rejection tests cover the failure behavior.

## Bounded aggregate supervision

gcontainers watch runc|crun SAMPLES INTERVAL_SECONDS emits newline-delimited aggregate JSON observations for 1–60 samples, 1–10 seconds between observations and a maximum 5-second census. No IDs, PIDs or paths are emitted; on an uncertain census it emits the incident sample and stops nonzero, on unavailable census it stops nonzero without inventing data. No persistent daemon, automatic recovery, health or instance identity guarantee. Test covers transition to pending incident and cancellation.
