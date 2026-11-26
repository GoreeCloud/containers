#!/usr/bin/env bash
set -euo pipefail
umask 077
[[ "$(id -u)" != 0 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
d="$(mktemp -d)"
trap 'rm -rf -- "$d"' EXIT
go build -o "$d/gcontainers" ./cmd/gcontainers
gc="$d/gcontainers"
mkdir -m 700 "$d/store" "$d/backups"
"$gc" volume create "$d/store" source --experimental >/dev/null
mkdir -m 700 "$d/store/source/nested"
printf '%s' 'synthetic family data' > "$d/store/source/nested/data"
"$gc" volume backup "$d/store" source "$d/backups" --experimental > "$d/backup.json"
mapfile -t result < <(python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["files"]==1 and not r["applicationConsistent"];print(r["directory"]);print(r["contentSHA256"])' < "$d/backup.json")
"$gc" volume restore "$d/store" restored "${result[0]}" "${result[1]}" --experimental > "$d/restore.json"
cmp "$d/store/source/nested/data" "$d/store/restored/nested/data"
if "$gc" volume restore "$d/store" source "${result[0]}" "${result[1]}" --experimental; then exit 1; fi
if "$gc" volume restore "$d/store" invalid "${result[0]}" "sha256:$(printf '%064d' 0)" --experimental; then exit 1; fi
test ! -e "$d/store/invalid"
"$gc" volume count "$d/store" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["count"]==2 and not r["mountVerified"]'
echo VOLUME_BACKUP_RESTORE_CLI_PASS
