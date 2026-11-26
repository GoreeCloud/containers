#!/usr/bin/env bash
# Explicit first-party isolated fixture; no stack apply or production data.
set -euo pipefail
umask 077
[[ "$#" == 1 && "$1" == --execute ]] || exit 2
[[ "$(id -u)" != 0 && "$(uname -m)" == x86_64 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
printf '%s  %s\n' 50dcc0b257d724fe07f3852c2dda52340223f038227c7572644eec870741223e /usr/local/bin/runc | sha256sum -c -
prepare="$(GC_SMOKE_RESOURCES=1 bash scripts/oci-smoke.sh --prepare-only)"
workspace="$(printf '%s\n' "$prepare" | sed -n 's/^Private fixture directory: //p')"
[[ "$workspace" =~ ^/tmp/gcontainers-smoke\.[A-Za-z0-9]+$ && -d "$workspace" && ! -L "$workspace" && "$(stat -c %u "$workspace")" == "$(id -u)" && "$(stat -c %a "$workspace")" == 700 ]]
trap 'echo "Incomplete synthetic stack observation: inspect $workspace; no forced cleanup" >&2' ERR
export XDG_RUNTIME_DIR="$workspace/runtime"
export PATH=/usr/local/bin:/usr/bin:/bin
gc="$workspace/gcontainers"
python3 - "$workspace/stack.json" <<'PY'
import json,sys
def service(n,dep):
 return dict(name=n,image="sha256:"+"a"*64,replicas=1,dependsOn=dep,resources=dict(cpuMilli=500,memoryBytes=67108864),security=dict(runAsNonRoot=True,readOnlyRootFilesystem=True,network="isolated"))
json.dump(dict(apiVersion="goreecloud.containers/v1alpha1",kind="Stack",metadata=dict(name="synthetic"),services=[service("db",[]),service("api",["db"])]),open(sys.argv[1],"w"))
PY
"$gc" stack instance-plan "$workspace/stack.json" > "$workspace/plan.json"
mapfile -t values < <(python3 - "$workspace/plan.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1]));print(p["planSHA256"]);print(p["startWaves"][0]["instances"][0]["id"])
PY
)
pin="${values[0]}";id="${values[1]}"
# This is the existing individually reviewed synthetic guest, not a native
# stack implementation. Its image/identity does not match the declaration.
"$gc" oci create --runtime runc --id "$id" --bundle "$workspace/bundle" --experimental
"$gc" oci start --runtime runc --id "$id" --experimental
"$gc" stack observe "$workspace/stack.json" runc --expected-plan-sha256 "$pin" > "$workspace/observed.json"
python3 - "$workspace/observed.json" <<'PY'
import json,sys
r=json.load(open(sys.argv[1]))
assert r["running"]==1 and r["missing"]==1 and r["expected"]==2 and not r["instanceIdentityVerified"] and not r["healthVerified"] and not r["deploymentAuthorized"]
assert r["instances"][1]["dependencyRunningCount"]==1 and r["instances"][1]["state"]=="missing"
PY
if "$gc" stack observe "$workspace/stack.json" runc --expected-plan-sha256 "sha256:$(printf '%064d' 0)"; then exit 1;fi
"$gc" oci stop --runtime runc --id "$id" --experimental
"$gc" oci delete --runtime runc --id "$id" --experimental
"$gc" stack observe "$workspace/stack.json" runc --expected-plan-sha256 "$pin" | python3 -c 'import json,sys;r=json.load(sys.stdin);assert r["missing"]==2 and r["running"]==0 and not r["recoveryRequired"]'
echo LIVE_STACK_OBSERVATION_PASS
