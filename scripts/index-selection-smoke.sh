#!/usr/bin/env bash
set -euo pipefail
umask 077
[[ "$(id -u)" != 0 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
d="$(mktemp -d)"
trap 'rm -rf -- "$d"' EXIT
go build -o "$d/gcontainers" ./cmd/gcontainers
gc="$d/gcontainers"
mkdir -m 700 "$d/staging"
python3 - "$d" <<'PY'
import hashlib,io,json,pathlib,sys,tarfile
p=pathlib.Path(sys.argv[1])
def pin(b):return "sha256:"+hashlib.sha256(b).hexdigest()
def desc(media,b):return {"mediaType":media,"digest":pin(b),"size":len(b)}
def write(name,media,v):
 b=json.dumps(v,separators=(",",":")).encode();(p/name).write_bytes(b);return desc(media,b)
items=[]
for arch in ("amd64","arm64"):
 out=io.BytesIO()
 with tarfile.open(fileobj=out,mode="w",format=tarfile.USTAR_FORMAT) as t:
  b=arch.encode();i=tarfile.TarInfo("platform");i.mode=0o600;i.size=len(b);t.addfile(i,io.BytesIO(b))
 b=out.getvalue();(p/(arch+"-layer")).write_bytes(b)
 c=write(arch+"-config","application/vnd.oci.image.config.v1+json",{"os":"linux","architecture":arch,"rootfs":{"type":"layers","diff_ids":[pin(b)]}})
 m=write(arch+"-manifest","application/vnd.oci.image.manifest.v1+json",{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":c,"layers":[desc("application/vnd.oci.image.layer.v1.tar",b)]})
 items.append(dict(m,platform={"os":"linux","architecture":arch}))
idx=write("index","application/vnd.oci.image.index.v1+json",{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":items})
(p/"pin").write_text(idx["digest"]);(p/"size").write_text(str(idx["size"]))
PY
for b in amd64-layer amd64-config amd64-manifest arm64-layer arm64-config arm64-manifest index; do "$gc" image import --store "$d/store" --file "$d/$b" --experimental >/dev/null; done
for arch in amd64 arm64; do
 "$gc" image select-index "$d/store" "$(cat "$d/pin")" "$(cat "$d/size")" "$arch" > "$d/selection"
 mapfile -t selected < <(python3 - "$arch" "$d/selection" <<'PY'
import json,sys
a,r=sys.argv[1],json.load(open(sys.argv[2]));assert r["architecture"]==a and r["metadataVerified"] and not r["layerContentsVerified"] and not r["executionAuthorized"]
print(r["manifest"]["digest"]);print(r["manifest"]["size"])
PY
)
 "$gc" image assemble-manifest "$d/store" "${selected[0]}" "${selected[1]}" "$d/staging" --experimental > "$d/result"
 python3 - "$arch" "$d/result" <<'PY'
import json,pathlib,sys
a,r=sys.argv[1],json.load(open(sys.argv[2]));assert r["diffIdsVerified"] and r["architecture"]==a and (pathlib.Path(r["directory"])/"platform").read_text()==a
PY
done
echo INDEX_SELECTION_ASSEMBLY_CLI_PASS
