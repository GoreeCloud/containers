#!/usr/bin/env bash
# Explicit first-party synthetic image, never production; no host allocation edits.
set -euo pipefail
umask 077
[[ ( "$#" == 1 || ( "$#" == 2 && ( "$2" == --persistent || "$2" == --persistent-soak || "$2" == --natural-exit || "$2" == --interrupted-stop || "$2" == --prelaunch-failure ) ) ) && "$1" == --execute && "$(id -u)" != 0 && "$(uname -m)" == x86_64 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
printf '%s  %s\n' 50dcc0b257d724fe07f3852c2dda52340223f038227c7572644eec870741223e /usr/local/bin/runc | sha256sum -c -
prepare="$(GC_SMOKE_RESOURCES=1 bash scripts/oci-smoke.sh --prepare-only)"
workspace="$(printf '%s\n' "$prepare" | sed -n 's/^Private fixture directory: //p')"
[[ "$workspace" =~ ^/tmp/gcontainers-smoke\.[A-Za-z0-9]+$ && -d "$workspace" && ! -L "$workspace" && "$(stat -c %u "$workspace")" == "$(id -u)" && "$(stat -c %a "$workspace")" == 700 ]]
echo "Private stack fixture: $workspace"
trap 'echo "Incomplete exercise: inspect $workspace; no forced cleanup" >&2' ERR
export PATH=/usr/local/bin:/usr/bin:/bin
export XDG_RUNTIME_DIR="$workspace/runtime"
gc="$workspace/gcontainers"
ticks=300
case "${2:-}" in --persistent-soak) ticks=900;; --natural-exit) ticks=120;; esac
gcc -static -O2 -Wall -Wextra -Werror -DGC_GUEST_ID=1000 -DGC_GUEST_TICKS="$ticks" scripts/fixtures/oci-smoke-guest.c -o "$workspace/bundle/rootfs/bin/probe"
mkdir -m 700 "$workspace/sessions" "$workspace/leases" "$workspace/other-sessions"
python3 - "$workspace" <<'PY'
import io,tarfile,json,hashlib,pathlib,sys
p=pathlib.Path(sys.argv[1]);exe=(p/"bundle/rootfs/bin/probe").read_bytes()
def pin(b):return "sha256:"+hashlib.sha256(b).hexdigest()
def desc(m,b):return {"mediaType":m,"digest":pin(b),"size":len(b)}
out=io.BytesIO()
with tarfile.open(fileobj=out,mode="w",format=tarfile.USTAR_FORMAT) as t:
 i=tarfile.TarInfo("bin/probe");i.size=len(exe);i.mode=0o755;t.addfile(i,io.BytesIO(exe))
layer=out.getvalue()
c=json.dumps({"os":"linux","architecture":"amd64","rootfs":{"type":"layers","diff_ids":[pin(layer)]}}).encode()
m=json.dumps({"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":desc("application/vnd.oci.image.config.v1+json",c),"layers":[desc("application/vnd.oci.image.layer.v1.tar",layer)]}).encode()
for n,b in (("layer",layer),("imageconfig",c),("manifest",m)):(p/n).write_bytes(b)
def service(n,replicas,dep):
 return dict(name=n,image=pin(m),replicas=replicas,dependsOn=dep,resources=dict(cpuMilli=500,memoryBytes=67108864),security=dict(runAsNonRoot=True,readOnlyRootFilesystem=True,network="isolated"))
json.dump(dict(apiVersion="goreecloud.containers/v1alpha1",kind="Stack",metadata=dict(name="synthetic"),services=[service("db",2,[]),service("api",1,["db"])]),open(p/"stack.json","w"))
PY
for n in layer imageconfig manifest;do "$gc" image import --store "$workspace/store" --file "$workspace/$n" --experimental >/dev/null;done
"$gc" stack instance-plan "$workspace/stack.json" > "$workspace/plan.json"
python3 - "$workspace" <<'PY'
import json,os,pwd,sys,pathlib,hashlib,copy
p=pathlib.Path(sys.argv[1]);plan=json.load(open(p/"plan.json"));config=json.load(open(p/"bundle/config.json"))
name=pwd.getpwuid(os.getuid()).pw_name
def allocation(f):
 r=[(int(start),int(count)) for owner,start,count in (l.split(":") for l in pathlib.Path(f).read_text().splitlines()) if owner in (name,str(os.getuid())) and int(count)>=3]
 assert len(r)==1 and r[0][0]>=65536
 return r[0][0]
u,g=allocation("/etc/subuid"),allocation("/etc/subgid")
def pin(b):return "sha256:"+hashlib.sha256(b).hexdigest()
entry=pin((p/"bundle/rootfs/bin/probe").read_bytes());size=(p/"manifest").stat().st_size
instances=[]
for n,i in enumerate(x for w in plan["startWaves"] for x in w["instances"]):
 c=copy.deepcopy(config);c["process"]["user"]={"uid":1000,"gid":1000}
 c["linux"]["uidMappings"].append(dict(containerID=1000,hostID=u+n,size=1))
 c["linux"]["gidMappings"].append(dict(containerID=1000,hostID=g+n,size=1))
 f=p/("config-"+str(n)+".json");raw=json.dumps(c).encode();f.write_bytes(raw)
 instances.append(dict(id=i["id"],manifestSize=size,configFile=str(f),configSHA256=pin(raw),entrypointSHA256=entry,whiteouts=False))
raw=json.dumps(dict(planSHA256=plan["planSHA256"],instances=instances)).encode();(p/"admission.json").write_bytes(raw);(p/"admission.pin").write_text(pin(raw))
PY
pin="$(cat "$workspace/admission.pin")"
"$gc" stack review-exercise "$workspace/stack.json" "$workspace/admission.json" "$pin" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["instances"]==3 and r["waves"]==2 and r["readOnly"] and not r["deploymentAuthorized"]'
if "$gc" stack exercise "$workspace/stack.json" "$workspace/admission.json" "sha256:$(printf '%064d' 0)" "$workspace/store" "$workspace/sessions" --experimental;then exit 1;fi
test -z "$(find "$workspace/sessions" -mindepth 1 -maxdepth 1 -print -quit)"
if [[ "${2:-}" == --prelaunch-failure ]];then
 # Induce a genuine pinned configuration preparation failure, before launch.
 cp "$workspace/config-0.json" "$workspace/config-0.saved"
 printf '\n' >> "$workspace/config-0.json"
 if "$gc" stack start-local "$workspace/stack.json" "$workspace/admission.json" "$pin" "$workspace/store" "$workspace/sessions" "$workspace/leases" --experimental > "$workspace/failed.json";then exit 1;fi
 mapfile -t failed < <(python3 - "$workspace/failed.json" <<'PYFAIL'
import json,sys
r=json.load(open(sys.argv[1]));assert r["persistent"] and r["recoveryReviewRequired"] and r["completedWaves"]==0
print(r["sessionDirectory"]);print(r["prelaunchSHA256"])
PYFAIL
 )
 session="${failed[0]}";prepin="${failed[1]}"
 test -f "$workspace/sessions/active-session"
 if "$gc" stack cancel-prelaunch "$session" "sha256:$(printf '%064d' 0)" --experimental;then exit 1;fi
 "$gc" stack review-prelaunch "$session" "$prepin" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["readOnly"] and r["emptyInventoryObserved"] and r["noLaunchIntentObserved"] and not r["reservationReleased"]'
 "$gc" stack cancel-prelaunch "$session" "$prepin" --experimental | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["reservationReleased"] and not r["recoveryReviewRequired"]'
 test -f "$session/prelaunch-cancelled";test ! -e "$workspace/sessions/active-session"
 mv "$workspace/config-0.saved" "$workspace/config-0.json"
 "$gc" stack start-local "$workspace/stack.json" "$workspace/admission.json" "$pin" "$workspace/store" "$workspace/sessions" "$workspace/leases" --experimental > "$workspace/restarted.json"
 mapfile -t restarted < <(python3 - "$workspace/restarted.json" <<'PYRESTART'
import json,sys
r=json.load(open(sys.argv[1]));assert r["allRunningObserved"];print(r["sessionDirectory"]);print(r["receiptSHA256"]);print(r["prelaunchSHA256"])
PYRESTART
 )
 if "$gc" stack cancel-prelaunch "${restarted[0]}" "${restarted[2]}" --experimental;then exit 1;fi
 "$gc" stack stop-local "${restarted[0]}" "${restarted[1]}" --experimental | python3 -c 'import json,sys;assert json.load(sys.stdin)["cleanupVerified"]'
 echo PRELAUNCH_FAILURE_RECOVERY_PASS
 exit 0
fi
if [[ "${2:-}" == --persistent || "${2:-}" == --persistent-soak || "${2:-}" == --natural-exit || "${2:-}" == --interrupted-stop ]];then
 "$gc" stack start-local "$workspace/stack.json" "$workspace/admission.json" "$pin" "$workspace/store" "$workspace/sessions" "$workspace/leases" --experimental > "$workspace/local.json"
 mapfile -t values < <(python3 - "$workspace/local.json" <<'PYLOCAL'
import json,sys
r=json.load(open(sys.argv[1]));assert r["persistent"] and r["allDelegatedEvidenceObserved"] and r["completedWaves"]==2 and not r["cleanupVerified"] and not r["deploymentAuthorized"]
print(r["sessionDirectory"]);print(r["receiptSHA256"])
PYLOCAL
 )
 session="${values[0]}";receipt="${values[1]}"
 "$gc" stack status-local "$session" "$receipt" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["running"]==3 and r["bindingsObserved"] and not r["recoveryReviewRequired"]'
 if [[ "${2:-}" == --interrupted-stop ]];then
 # A final-event-only synthetic log must never authorize status or stop.
 cp -- "$session/events.ndjson" "$workspace/events.saved"
 tail -n 1 "$workspace/events.saved" > "$session/events.ndjson"
 if "$gc" stack status-local "$session" "$receipt";then exit 1;fi
 if "$gc" stack stop-local "$session" "$receipt" --experimental;then exit 1;fi
 test ! -e "$session/stop-intent"
 cat "$workspace/events.saved" > "$session/events.ndjson"
 "$gc" stack status-local "$session" "$receipt" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["running"]==3 and r["bindingsObserved"]'
 echo PERSISTENT_START_LOG_REJECTION_PASS
 # Fixture-only interruption simulation: persist exact stop intent, then
 # complete the first reverse wave through the existing isolated OCI CLI.
 # No pending marker is removed and no host or unrelated workload is used.
 if "$gc" stack review-stop "$session" "$receipt";then exit 1;fi
 if "$gc" stack resume-stop "$session" "$receipt" --experimental;then exit 1;fi
 printf '%s\n' "$receipt" > "$session/stop-intent"
 id="$(python3 - "$session/local-receipt.json" <<'PYINT'
import json,sys
r=json.load(open(sys.argv[1]));assert len(r["stopWaves"][0])==1
print(r["bindings"][r["stopWaves"][0][0]]["id"])
PYINT
 )"
 XDG_RUNTIME_DIR="$session/runtime" "$gc" oci stop --runtime runc --id "$id" --experimental
 XDG_RUNTIME_DIR="$session/runtime" "$gc" oci delete --runtime runc --id "$id" --experimental
 if "$gc" stack stop-local "$session" "$receipt" --experimental;then exit 1;fi
 if "$gc" stack review-stop "$session" "sha256:$(printf '%064d' 0)";then exit 1;fi
 "$gc" stack review-stop "$session" "$receipt" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["readOnly"] and r["stopResumeChecksPassed"] and r["recoveryReviewRequired"] and r["running"]==2 and r["missingProcessAbsent"]==1 and not r["cleanupVerified"] and not r["deploymentAuthorized"]'
 test -f "$workspace/sessions/active-session"
 test ! -e "$session/cleanup-verified"
 echo INTERRUPTED_STOP_READ_ONLY_REVIEW_PASS
 "$gc" stack resume-stop "$session" "$receipt" --experimental | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["missingProcessAbsent"]==1 and r["cleanupVerified"] and not r["recoveryReviewRequired"]'
 test ! -e "$workspace/sessions/active-session"
 test -z "$(find "$workspace/leases" -maxdepth 1 -name 'lease-*.json' -print -quit)"
 echo INTERRUPTED_STOP_RESUME_PASS
 exit 0
 fi
 if [[ "${2:-}" == --natural-exit ]];then
 for n in {1..25};do
  "$gc" stack status-local "$session" "$receipt" > "$workspace/status.json"
  stopped="$(python3 -c 'import json,sys;print(json.load(sys.stdin)["stopped"])' < "$workspace/status.json")"
  [[ "$stopped" == 3 ]] && break
  sleep 1
 done
 [[ "$stopped" == 3 ]]
 "$gc" stack stop-local "$session" "$receipt" --experimental | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["cleanupVerified"] and not r["recoveryReviewRequired"]'
 test ! -e "$workspace/sessions/active-session"
 test -z "$(find "$workspace/leases" -maxdepth 1 -name 'lease-*.json' -print -quit)"
 echo NATURAL_EXIT_CLEANUP_PASS
 exit 0
 fi
 if [[ "${2:-}" == --persistent-soak ]];then
  for n in {1..6};do
   sleep 10
   "$gc" stack status-local "$session" "$receipt" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["running"]==3 and r["bindingsObserved"] and not r["recoveryReviewRequired"]'
   echo "SOAK_SAMPLE_$n"
  done
  echo SIXTY_SECOND_STACK_SOAK_PASS
 fi
 if "$gc" stack start-local "$workspace/stack.json" "$workspace/admission.json" "$pin" "$workspace/store" "$workspace/sessions" "$workspace/leases" --experimental;then exit 1;fi
 if "$gc" stack start-local "$workspace/stack.json" "$workspace/admission.json" "$pin" "$workspace/store" "$workspace/other-sessions" "$workspace/leases" --experimental;then exit 1;fi
 test ! -e "$workspace/other-sessions/active-session"
 if "$gc" stack stop-local "$session" "sha256:$(printf '%064d' 0)" --experimental;then exit 1;fi
 "$gc" stack status-local "$session" "$receipt" | python3 -c 'import json,sys;assert json.load(sys.stdin)["running"]==3'
 "$gc" stack stop-local "$session" "$receipt" --experimental | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["cleanupVerified"] and not r["recoveryReviewRequired"]'
 test ! -e "$workspace/sessions/active-session"
 test -z "$(find "$workspace/leases" -maxdepth 1 -name 'lease-*.json' -print -quit)"
 test -f "$session/cleanup-verified"
 if "$gc" stack stop-local "$session" "$receipt" --experimental;then exit 1;fi
 # A fresh generation can reuse the same dedicated private slot after cleanup.
 "$gc" stack start-local "$workspace/stack.json" "$workspace/admission.json" "$pin" "$workspace/store" "$workspace/sessions" "$workspace/leases" --experimental > "$workspace/next.json"
 mapfile -t next < <(python3 - "$workspace/next.json" <<'PYNEXT'
import json,sys
r=json.load(open(sys.argv[1]));print(r["sessionDirectory"]);print(r["receiptSHA256"])
PYNEXT
 )
 [[ "${next[0]}" != "$session" && "${next[1]}" != "$receipt" ]]
 "$gc" stack stop-local "${next[0]}" "${next[1]}" --experimental | python3 -c 'import json,sys;assert json.load(sys.stdin)["cleanupVerified"]'
 test ! -e "$workspace/sessions/active-session"
 test -z "$(find "$workspace/leases" -maxdepth 1 -name 'lease-*.json' -print -quit)"
 echo PERSISTENT_LOCAL_STACK_PASS
 exit 0
fi
"$gc" stack exercise "$workspace/stack.json" "$workspace/admission.json" "$pin" "$workspace/store" "$workspace/sessions" --experimental > "$workspace/exercise.json"
python3 - "$workspace" <<'PY'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1]);r=json.load(open(p/"exercise.json"));assert r["prepared"]==3 and r["completedWaves"]==2 and r["completedStopWaves"]==2 and r["allRunningObserved"] and r["allDelegatedEvidenceObserved"] and r["cleanupVerified"] and not r["recoveryReviewRequired"] and not r["deploymentAuthorized"]
d=pathlib.Path(r["sessionDirectory"]);assert d.parent==p/"sessions"
receipt=json.load(open(d/"receipt.json"));ids=receipt["runtimeIDs"];assert len(set(ids.values()))==3 and all(k!=v for k,v in ids.items())
events=[json.loads(line) for line in (d/"events.ndjson").read_text().splitlines()]
assert events[-1]["phase"]=="cleanup-verified" and len([x for x in events if x["phase"]=="running-observed"])==2
root=d/"runtime/goreecloud-containers";assert sorted(x.name for x in root.iterdir())==["journal","locks"] and not list((root/"journal").iterdir())
PY
echo NATIVE_STACK_EXERCISE_PASS
