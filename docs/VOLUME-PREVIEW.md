# Private volume store preview

The existing storage parent must be an absolute caller-owned mode-0700 directory, not a symlink. `gcontainers volume create PRIVATE_STORE ID --experimental` creates a named private directory exclusively. `gcontainers volume inspect PRIVATE_STORE ID` checks metadata without reading payloads. `gcontainers volume count PRIVATE_STORE` checks the bounded directory inventory and returns an aggregate count. Duplicate creation refuses to overwrite data; there is no delete or reset operation. Tests preserve synthetic data across independent CLI invocations and reject path traversal, symlinks and unsafe inventory. This is persistent storage-directory management, not OCI mount attachment, filesystem quotas, encryption, backup, or container access verification. Workload mount admission remains closed.

The private store now cooperatively serializes creation with an exclusive directory-descriptor flock; inspection and counting use shared locks. Creation refuses unsafe inventory or a full 1,024-directory store. Concurrent final-slot and cancellation regressions passed. No payload enumeration, byte quota, mount, or same-UID hostile-writer protection.

## Quiesced private data backup and restore

gcontainers volume backup PRIVATE_STORE ID PRIVATE_BACKUP_PARENT --experimental creates a new private copy of an explicitly selected volume. gcontainers volume restore PRIVATE_STORE NEW_ID BACKUP_DIR SHA256 --experimental verifies the same canonical content pin while copying to an exclusively created new ID. Only caller-owned private regular files and directories, no hardlinks, symlinks, special files, privilege bits or open-file mount crossings; normalized 0600/0700 modes, at most 4096 entries/256 MiB. Source and target must be separate trees. Cooperating volume creation and restore respect the existing count lock; copying cannot freeze an application or uncooperative writer. Failed copies remove only their newly created matching directory. No existing volume overwrite, off-host transfer, encryption, live app consistency or mount authorization. Quiesce and independently protect backups before relying on them. Synthetic round-trip, wrong pin, no-overwrite and symlink rejection tests passed.

The repeatable synthetic CLI fixture scripts/volume-backup-smoke.sh passed on the laptop and is included in protected CI; no real family data was accessed.

## Read-only backup verification

gcontainers volume verify-backup BACKUP_DIR SHA256 recomputes the identical canonical normalized payload digest through the existing safe traversal without allocating a restore directory or writing payload. Detects tampering and wrong pins, checks source root identity, enforces existing size/entry/mount bounds and emits only aggregate counts and digest. Does not attest live application consistency or immutable backup storage. The CLI fixture verifies before restore.
