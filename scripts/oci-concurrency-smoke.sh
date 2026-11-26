#!/usr/bin/env bash
# Two first-party, bounded, isolated synthetic guests sharing only the private
# runtime metadata root. Explicit nonproduction execution; no host changes.
set -euo pipefail
umask 077
[[ "$#" -eq 1 && "$1" == --execute ]] || { echo "usage: scripts/oci-concurrency-smoke.sh --execute" >&2; exit 2; }
[[ "$(id -u)" != 0 && "$(id -g)" != 0 && "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
[[ -x /usr/local/bin/runc && "$(stat -c %u /usr/local/bin/runc)" == 0 ]]
printf '%s  %s\n' 50dcc0b257d724fe07f3852c2dda52340223f038227c7572644eec870741223e /usr/local/bin/runc | sha256sum -c -
prepare="$(GC_SMOKE_RESOURCES=1 bash scripts/oci-smoke.sh --prepare-only)"
workspace="$(printf '%s\n' "$prepare" | sed -n 's/^Private fixture directory: //p')"
[[ "$workspace" =~ ^/tmp/gcontainers-smoke\.[A-Za-z0-9]+$ && -d "$workspace" && ! -L "$workspace" && "$(stat -c %u "$workspace")" == "$(id -u)" && "$(stat -c %a "$workspace")" == 700 ]]
printf 'Private parallel fixture: %s\n' "$workspace"
trap 'echo "Parallel fixture incomplete: review private evidence at $workspace; no forced cleanup" >&2' ERR
export XDG_RUNTIME_DIR="$workspace/runtime"
export PATH=/usr/local/bin:/usr/bin:/bin
export GC_SMOKE_RESOURCES=1
gc="$workspace/gcontainers"
mkdir -m 700 "$workspace/snapshots"
for id in worker-a worker-b; do
  "$gc" snapshot "$workspace/bundle" "$workspace/snapshots" --experimental > "$workspace/$id-snapshot.json"
done
a="$(python3 -c 'import json,sys;print(json.load(sys.stdin)["directory"])' < "$workspace/worker-a-snapshot.json")"
b="$(python3 -c 'import json,sys;print(json.load(sys.stdin)["directory"])' < "$workspace/worker-b-snapshot.json")"
[[ "$a" != "$b" ]]
# Wait for both distinct operations, even when one fails.
join_two() {
  local failed=0
  wait "$1" || failed=1
  wait "$2" || failed=1
  return "$failed"
}
"$gc" oci create --runtime runc --id worker-a --bundle "$a" --experimental & p1=$!
"$gc" oci create --runtime runc --id worker-b --bundle "$b" --experimental & p2=$!
join_two "$p1" "$p2"
"$gc" oci start --runtime runc --id worker-a --experimental & p1=$!
"$gc" oci start --runtime runc --id worker-b --experimental & p2=$!
join_two "$p1" "$p2"
sleep 1
for id in worker-a worker-b; do
  "$gc" oci state --runtime runc --id "$id" > "$workspace/$id-running.json"
done
python3 - "$workspace" <<'PY'
import json,os,pathlib,sys
p=pathlib.Path(sys.argv[1]);states=[json.load(open(p/f"worker-{x}-running.json")) for x in ("a","b")]
assert all(s["status"]=="running" and s["pid"]>0 for s in states)
assert states[0]["pid"]!=states[1]["pid"]
for n in ("user","mnt","pid","net","ipc","uts"):
    f=[os.stat(f"/proc/{pid}/ns/{n}") for pid in ("self",states[0]["pid"],states[1]["pid"])]
    assert len({(x.st_dev,x.st_ino) for x in f})==3,n
for s in states:
    pid=s["pid"]
    status=dict(line.rstrip().split(":",1) for line in open(f"/proc/{pid}/status") if ":" in line)
    assert status["Name"].strip()=="probe" and status["NoNewPrivs"].strip()=="1" and status["Seccomp"].strip()=="2"
    assert status["Uid"].split()==[str(os.getuid())]*4 and status["Gid"].split()==[str(os.getgid())]*4
    assert all(int(status[k].strip(),16)==0 for k in ("CapInh","CapPrm","CapEff","CapBnd","CapAmb"))
    rows=pathlib.Path(f"/proc/{pid}/cgroup").read_text().splitlines()
    paths=[line[3:] for line in rows if line.startswith("0::")]
    assert len(paths)==1
    cg=pathlib.Path("/sys/fs/cgroup").joinpath(paths[0].lstrip("/")).resolve(strict=True)
    assert cg.is_relative_to("/sys/fs/cgroup") and cg.name==s["id"]
    assert (cg/"cpu.max").read_text().split()==["50000","100000"]
    assert (cg/"memory.max").read_text().strip()=="67108864"
    assert (cg/"pids.max").read_text().strip()=="32" and str(pid) in (cg/"cgroup.procs").read_text().split()
print("TWO_GUEST_SIX_NAMESPACE_AND_CGROUP_PASS")
PY
# Observers run concurrently with graceful termination; no forced signals.
"$gc" wait runc worker-a 15 > "$workspace/worker-a-wait.json" & w1=$!
"$gc" wait runc worker-b 15 > "$workspace/worker-b-wait.json" & w2=$!
"$gc" oci stop --runtime runc --id worker-a --experimental & p1=$!
"$gc" oci stop --runtime runc --id worker-b --experimental & p2=$!
join_two "$p1" "$p2"
join_two "$w1" "$w2"
python3 - "$workspace" <<'PY'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1])
for id in ("worker-a","worker-b"):
    r=json.load(open(p/f"{id}-wait.json"))
    assert r["id"]==id and r["stopped"] and r["readOnly"] and not r["exitCodeVerified"] and not r["instanceIdentityVerified"]
print("CONCURRENT_WAIT_STOP_PASS")
PY
"$gc" oci delete --runtime runc --id worker-a --experimental & p1=$!
"$gc" oci delete --runtime runc --id worker-b --experimental & p2=$!
join_two "$p1" "$p2"
state="$XDG_RUNTIME_DIR/goreecloud-containers"
for id in worker-a worker-b; do
  test ! -e "$state/$id" && test ! -L "$state/$id"
done
"$gc" oci pending-summary | python3 -c 'import json,sys;assert json.load(sys.stdin)["pendingCount"]==0'
trap - ERR
echo OCI_CONCURRENT_LIFECYCLE_PASS
