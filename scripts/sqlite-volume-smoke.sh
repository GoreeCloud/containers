#!/usr/bin/env bash
# Synthetic host-side database backup acceptance, not a mounted guest volume.
set -euo pipefail
umask 077
[[ "$#" == 0 && "$(id -u)" != 0 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
d="$(mktemp -d /tmp/gcontainers-sqlite.XXXXXXXX)"
echo "Private database fixture: $d"
go build -o "$d/gcontainers" ./cmd/gcontainers
gc="$d/gcontainers"
mkdir -m 700 "$d/store" "$d/backups"
"$gc" volume create "$d/store" source --experimental >/dev/null
python3 - "$d/store/source/state.db" <<'PY'
import sqlite3,sys,pathlib
p=pathlib.Path(sys.argv[1]);c=sqlite3.connect(p)
try:
 assert c.execute("PRAGMA journal_mode=WAL").fetchone()[0]=="wal"
 c.execute("PRAGMA synchronous=FULL")
 c.execute("CREATE TABLE items(id INTEGER PRIMARY KEY, value INTEGER NOT NULL CHECK(value>=0))")
 c.execute("CREATE TABLE history(seq INTEGER PRIMARY KEY, item INTEGER NOT NULL REFERENCES items(id))")
 c.execute("PRAGMA foreign_keys=ON")
 c.executemany("INSERT INTO items VALUES (?,?)",[(n,n*7) for n in range(1,101)])
 c.commit()
 for i in range(1,11):
  with c:
   c.execute("UPDATE items SET value=value+1 WHERE id=?",(i,))
   c.execute("INSERT INTO history VALUES (?,?)",(i,i))
 c.execute("BEGIN")
 c.execute("UPDATE items SET value=999999 WHERE id=1")
 c.rollback()
 assert c.execute("PRAGMA integrity_check").fetchall()==[("ok",)]
 assert c.execute("PRAGMA foreign_key_check").fetchall()==[]
 assert c.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchone()[0]==0
finally:
 c.close()
assert not pathlib.Path(str(p)+"-wal").exists()
assert not pathlib.Path(str(p)+"-shm").exists()
PY
"$gc" volume backup "$d/store" source "$d/backups" --experimental > "$d/backup.json"
mapfile -t result < <(python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["files"]==1 and not r["applicationConsistent"] and not r["mountVerified"];print(r["directory"]);print(r["contentSHA256"])' < "$d/backup.json")
"$gc" volume verify-backup "${result[0]}" "${result[1]}" >/dev/null
"$gc" volume restore "$d/store" restored "${result[0]}" "${result[1]}" --experimental > "$d/restore.json"
"$gc" volume fingerprint "$d/store" source "${result[1]}" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["digestMatches"] and r["readOnly"] and not r["mountVerified"]'
"$gc" volume fingerprint "$d/store" restored "${result[1]}" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["digestMatches"] and r["files"]==1'

cmp "$d/store/source/state.db" "$d/store/restored/state.db"
python3 - "$d/store" <<'PY'
import sqlite3,sys,pathlib
p=pathlib.Path(sys.argv[1])
def read(name):
 c=sqlite3.connect((p/name/"state.db").as_uri()+"?mode=ro&immutable=1",uri=True)
 try:
  assert c.execute("PRAGMA integrity_check").fetchall()==[("ok",)]
  assert c.execute("PRAGMA foreign_key_check").fetchall()==[]
  assert c.execute("SELECT COUNT(*),SUM(value) FROM items").fetchone()==(100,35360)
  assert c.execute("SELECT * FROM history ORDER BY seq").fetchall()==[(n,n) for n in range(1,11)]
  assert c.execute("SELECT value FROM items WHERE id=1").fetchone()==(8,)
 finally:c.close()
read("source");read("restored")
c=sqlite3.connect(p/"restored/state.db")
with c:
 c.execute("UPDATE items SET value=value+100 WHERE id=1")
c.close()
c=sqlite3.connect(p/"restored/state.db")
assert c.execute("SELECT value FROM items WHERE id=1").fetchone()==(108,)
c.close()
read("source")
PY
"$gc" volume verify-backup "${result[0]}" "${result[1]}" >/dev/null
if "$gc" volume fingerprint "$d/store" restored "${result[1]}";then exit 1;fi
"$gc" volume fingerprint "$d/store" source "${result[1]}" >/dev/null
echo SQLITE_VOLUME_FINGERPRINT_PASS
echo SQLITE_QUIESCED_BACKUP_RESTORE_PASS
