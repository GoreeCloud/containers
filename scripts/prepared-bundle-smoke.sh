#!/usr/bin/env bash
# First-party static fixture; no external image, registry or host change.
set -euo pipefail
umask 077
[[ ( "$#" == 1 || ( "$#" == 2 && "$2" == --whiteouts ) ) && ( "$1" == --execute || "$1" == --prepare-only ) ]] || exit 2
whiteouts=()
if [[ "$#" == 2 ]]; then whiteouts=(--whiteouts); fi
mode="$1"
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
prepare="$(GC_SMOKE_RESOURCES=1 bash scripts/oci-smoke.sh --prepare-only)"
workspace="$(printf '%s\n' "$prepare" | sed -n 's/^Private fixture directory: //p')"
[[ "$workspace" =~ ^/tmp/gcontainers-smoke\.[A-Za-z0-9]+$ && -d "$workspace" && ! -L "$workspace" && "$(stat -c %u "$workspace")" == "$(id -u)" && "$(stat -c %a "$workspace")" == 700 ]]
gc="$workspace/gcontainers"
export XDG_RUNTIME_DIR="$workspace/runtime"
export PATH=/usr/local/bin:/usr/bin:/bin
mkdir -m 700 "$workspace/prepared"
python3 - "$workspace" "${2:-}" <<'PY'
import io,tarfile,json,hashlib,pathlib,sys,gzip
p=pathlib.Path(sys.argv[1]);exe=(p/"bundle/rootfs/bin/probe").read_bytes()
def pin(b):return "sha256:"+hashlib.sha256(b).hexdigest()
def desc(m,b):return {"mediaType":m,"digest":pin(b),"size":len(b)}
def tar(entries):
 out=io.BytesIO()
 with tarfile.open(fileobj=out,mode="w",format=tarfile.USTAR_FORMAT) as t:
  for name,data in entries:
   i=tarfile.TarInfo(name);i.size=len(data);i.mode=0o755 if name=="bin/probe" else 0o600;t.addfile(i,io.BytesIO(data))
 return out.getvalue()
layer=tar([("bin/probe",exe)])
layers=[desc("application/vnd.oci.image.layer.v1.tar",layer)];diffs=[pin(layer)]
(p/"layer").write_bytes(layer)
if sys.argv[2]=="--whiteouts":
 lower=tar([("bin/probe",b"obsolete"),("etc/old",b"old"),("keep",b"keep")])
 upper=tar([("bin/probe",exe),("bin/.wh.probe",b""),("etc/.wh..wh..opq",b""),("etc/new",b"new")])
 gz=gzip.compress(upper,mtime=0)
 layers=[desc("application/vnd.oci.image.layer.v1.tar",lower),desc("application/vnd.oci.image.layer.v1.tar+gzip",gz)]
 diffs=[pin(lower),pin(upper)]
 (p/"layer").write_bytes(lower);(p/"upper").write_bytes(gz)
c=json.dumps({"os":"linux","architecture":"amd64","rootfs":{"type":"layers","diff_ids":diffs}}).encode()
m=json.dumps({"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":desc("application/vnd.oci.image.config.v1+json",c),"layers":layers}).encode()
for n,b in (("imageconfig",c),("manifest",m)):(p/n).write_bytes(b)
(p/"pins").write_text("\n".join([pin(m),str(len(m)),pin((p/"bundle/config.json").read_bytes()),pin(exe)])+"\n")
PY
for n in layer imageconfig manifest; do "$gc" image import --store "$workspace/store" --file "$workspace/$n" --experimental >/dev/null; done
if [[ "${#whiteouts[@]}" == 1 ]]; then "$gc" image import --store "$workspace/store" --file "$workspace/upper" --experimental >/dev/null; fi
mapfile -t pins < "$workspace/pins"
if "$gc" image prepare-bundle "$workspace/store" "${pins[0]}" "${pins[1]}" "$workspace/bundle/config.json" "${pins[2]}" "sha256:$(printf '%064d' 0)" "$workspace/prepared" "${whiteouts[@]}" --experimental >/dev/null 2>&1; then echo "wrong entrypoint pin accepted";exit 1;fi
test "$(find "$workspace/prepared" -mindepth 1 -maxdepth 1 | wc -l)" -eq 0
"$gc" image prepare-bundle "$workspace/store" "${pins[0]}" "${pins[1]}" "$workspace/bundle/config.json" "${pins[2]}" "${pins[3]}" "$workspace/prepared" "${whiteouts[@]}" --experimental > "$workspace/prepared.json"
dir="$(python3 -c 'import json,sys,pathlib;s=json.load(sys.stdin);assert s["layoutVerified"] and s["diffIdsVerified"] and not s["executionAuthorized"];assert (pathlib.Path(s["directory"])/"rootfs/bin/probe").stat().st_mode&0o777==0o500;print(s["directory"])' < "$workspace/prepared.json")"
if [[ "${#whiteouts[@]}" == 1 ]]; then
 python3 - "$workspace/prepared.json" <<'PY2'
import json,pathlib,sys
r=json.load(open(sys.argv[1]));p=pathlib.Path(r["directory"])/"rootfs"
assert r["whiteoutsApplied"]==2 and (p/"etc/new").read_bytes()==b"new" and (p/"keep").read_bytes()==b"keep"
assert not (p/"etc/old").exists() and not (p/"bin/.wh.probe").exists()
PY2
 echo PREPARED_WHITEOUT_BUNDLE_PASS
fi
echo PREPARED_IMAGE_BUNDLE_PASS
"$gc" preflight "$dir"
if [[ "$mode" == --prepare-only ]]; then echo PREPARED_IMAGE_NO_RUNTIME_PASS; exit 0; fi
printf '%s  %s\n' 50dcc0b257d724fe07f3852c2dda52340223f038227c7572644eec870741223e /usr/local/bin/runc | sha256sum -c -
id=prepared-probe
"$gc" oci create --runtime runc --id "$id" --bundle "$dir" --experimental
"$gc" oci start --runtime runc --id "$id" --experimental
sleep 1
"$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys;s=json.load(sys.stdin);assert s["status"]=="running" and s["pid"]>0'
"$gc" oci stop --runtime runc --id "$id" --experimental
"$gc" oci delete --runtime runc --id "$id" --experimental
"$gc" oci pending-summary | python3 -c 'import json,sys;assert json.load(sys.stdin)["pendingCount"]==0'
echo PREPARED_IMAGE_LIFECYCLE_PASS
