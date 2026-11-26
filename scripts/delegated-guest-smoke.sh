#!/usr/bin/env bash
# Only the reviewed first-party synthetic guest; no account or host policy edits.
set -euo pipefail
umask 077
[[ "$#" == 1 && ( "$1" == --prepare-only || "$1" == --execute ) ]]
[[ "$(id -u)" != 0 && "$(uname -m)" == x86_64 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
prepare="$(GC_SMOKE_RESOURCES=1 bash scripts/oci-smoke.sh --prepare-only)"
workspace="$(printf '%s\n' "$prepare" | sed -n 's/^Private fixture directory: //p')"
[[ "$workspace" =~ ^/tmp/gcontainers-smoke\.[A-Za-z0-9]+$ && -d "$workspace" && ! -L "$workspace" && "$(stat -c %u "$workspace")" == "$(id -u)" && "$(stat -c %a "$workspace")" == 700 ]]
trap 'echo "Delegated fixture incomplete: preserve private evidence at $workspace; no forced cleanup" >&2' ERR
export XDG_RUNTIME_DIR="$workspace/runtime"
export PATH=/usr/local/bin:/usr/bin:/bin
gc="$workspace/gcontainers"
python3 - "$workspace/bundle" <<'PY'
import json,os,pwd,sys,pathlib
p=pathlib.Path(sys.argv[1]);u=os.getuid();g=os.getgid();name=pwd.getpwuid(u).pw_name
def first(f):
 ranges=[]
 for line in pathlib.Path(f).read_text().splitlines():
  owner,start,count=line.split(":")
  if owner in (name,str(u)) and int(count)>0:ranges.append(int(start))
 assert len(ranges)==1 and ranges[0]>=65536
 return ranges[0]
c=p/"config.json";s=json.loads(c.read_text());s["process"]["user"]={"uid":1000,"gid":1000}
s["linux"]["uidMappings"].append(dict(containerID=1000,hostID=first("/etc/subuid"),size=1))
s["linux"]["gidMappings"].append(dict(containerID=1000,hostID=first("/etc/subgid"),size=1))
c.write_text(json.dumps(s));c.chmod(0o600)
for d in (p/"rootfs").rglob("*"):
 if d.is_dir():d.chmod(0o755)
(p/"rootfs").chmod(0o755)
PY
gcc -static -O2 -Wall -Wextra -Werror -DGC_GUEST_ID=1000 scripts/fixtures/oci-smoke-guest.c -o "$workspace/bundle/rootfs/bin/probe"
chmod 0555 "$workspace/bundle/rootfs/bin/probe"
"$gc" preflight-delegated "$workspace/bundle" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["mappingVerified"] and r["layoutVerified"] and r["guestNonRoot"] and not r["runtimeInvoked"] and not r["deploymentAuthorized"]'
echo DELEGATED_PREFLIGHT_PASS
# Refuse host-visible bundle permissions and extra hardlinked entries.
chmod 0700 "$workspace/bundle/rootfs/bin"
if "$gc" preflight-delegated "$workspace/bundle" >/dev/null 2>&1; then echo "inaccessible guest directory accepted";exit 1;fi
chmod 0755 "$workspace/bundle/rootfs/bin"
ln "$workspace/bundle/rootfs/bin/probe" "$workspace/bundle/rootfs/duplicate"
if "$gc" preflight-delegated "$workspace/bundle" >/dev/null 2>&1; then echo "hardlinked guest file accepted";exit 1;fi
rm "$workspace/bundle/rootfs/duplicate"

mkdir -m 700 "$workspace/snapshots"
"$gc" snapshot-delegated "$workspace/bundle" "$workspace/snapshots" --experimental > "$workspace/snapshot.json"
snapshot="$(python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["delegatedGuest"] and not r["immutable"] and not r["executionAuthorized"];print(r["directory"])' < "$workspace/snapshot.json")"
test "$(stat -c %a "$snapshot")" == 700
test "$(stat -c %a "$snapshot/config.json")" == 400
test "$(stat -c %a "$snapshot/rootfs")" == 755
test "$(stat -c %a "$snapshot/rootfs/bin/probe")" == 555
"$gc" preflight-delegated "$snapshot" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["mappingVerified"] and r["layoutVerified"] and not r["runtimeInvoked"]'
echo DELEGATED_PRIVATE_SNAPSHOT_PASS

[[ "$1" == --execute ]] || exit 0
printf '%s  %s\n' 50dcc0b257d724fe07f3852c2dda52340223f038227c7572644eec870741223e /usr/local/bin/runc | sha256sum -c -
id="delegated-probe"
if "$gc" oci create --runtime runc --id "$id" --bundle "$snapshot" --experimental;then echo "default policy admitted delegated mapping";exit 1;fi
"$gc" oci create --runtime runc --id "$id" --bundle "$snapshot" --delegated-guest --experimental
"$gc" oci start --runtime runc --id "$id" --experimental
sleep 1
"$gc" oci state --runtime runc --id "$id" > "$workspace/state.json"
python3 - "$workspace" <<'PY'
import json,sys,pathlib
p=pathlib.Path(sys.argv[1]);r=json.load(open(p/"state.json"));s=json.load(open(p/"bundle/config.json"))
assert r["status"]=="running" and r["pid"]>0
status=dict(line.split(":",1) for line in pathlib.Path("/proc",str(r["pid"]),"status").read_text().splitlines() if ":" in line)
assert status["Name"].strip()=="probe"
assert status["Uid"].split()==[str(s["linux"]["uidMappings"][1]["hostID"])]*4
assert status["Gid"].split()==[str(s["linux"]["gidMappings"][1]["hostID"])]*4
assert status["NoNewPrivs"].strip()=="1" and status["Seccomp"].strip()=="2"
assert all(int(status[k],16)==0 for k in ("CapInh","CapPrm","CapEff","CapBnd","CapAmb"))
assert status["NSpid"].split()[-1]=="1"
print("DELEGATED_GUEST_IDENTITY_PASS")
PY
 "$gc" delegated-observe runc "$id" "$snapshot" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert all(r[k] for k in ("runningObserved","subordinateIdentityObserved","pidNamespaceObserved","zeroCapabilitiesObserved","noNewPrivilegesObserved","seccompFilterObserved","resourceLimitsObserved")) and not r["bundleIdentityVerified"] and not r["deploymentAuthorized"]'
echo LIVE_DELEGATED_EVIDENCE_PASS
"$gc" oci stop --runtime runc --id "$id" --experimental
"$gc" oci delete --runtime runc --id "$id" --experimental
"$gc" oci pending-summary | python3 -c 'import json,sys;assert json.load(sys.stdin)["pendingCount"]==0'
echo DELEGATED_GUEST_LIFECYCLE_PASS
