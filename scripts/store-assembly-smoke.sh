#!/usr/bin/env bash
# Inert first-party data only. No runtime, network, privileged commands or secrets.
set -euo pipefail
[[ "$(id -u)" != 0 ]] || { echo "nonroot fixture required" >&2; exit 1; }
workspace="$(mktemp -d)"
chmod 700 "$workspace"
trap 'rm -rf -- "$workspace"' EXIT
gc="$workspace/gcontainers"
go build -o "$gc" ./cmd/gcontainers
mkdir -m 700 "$workspace/staging"
python3 - "$workspace" <<'PY'
import gzip, hashlib, io, json, pathlib, sys, tarfile
p=pathlib.Path(sys.argv[1])
def make(name,data):
    b=io.BytesIO()
    with tarfile.open(fileobj=b, mode="w", format=tarfile.USTAR_FORMAT) as t:
        entry=tarfile.TarInfo(name);entry.mode=0o600;entry.size=len(data)
        t.addfile(entry,io.BytesIO(data))
    return b.getvalue()
def digest(data):return "sha256:"+hashlib.sha256(data).hexdigest()
def descriptor(media,data):return {"mediaType":media,"digest":digest(data),"size":len(data)}
layer1=make("etc/data",b"old")
layer2=make("etc/data",b"new")
compressed=gzip.compress(layer2,mtime=0)
config=json.dumps({"os":"linux","architecture":"amd64","rootfs":{"type":"layers","diff_ids":[digest(layer1),digest(layer2)]}},separators=(",",":")).encode()
manifest=json.dumps({"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":descriptor("application/vnd.oci.image.config.v1+json",config),"layers":[descriptor("application/vnd.oci.image.layer.v1.tar",layer1),descriptor("application/vnd.oci.image.layer.v1.tar+gzip",compressed)]},separators=(",",":")).encode()
for name,data in (("layer1",layer1),("layer2",compressed),("config",config),("manifest",manifest)):(p/name).write_bytes(data)
(p/"pin").write_text(digest(manifest))
(p/"size").write_text(str(len(manifest)))
PY
for blob in layer1 layer2 config manifest; do
  "$gc" image import --store "$workspace/store" --file "$workspace/$blob" --experimental > "$workspace/$blob.json"
done
pin="$(cat "$workspace/pin")"
size="$(cat "$workspace/size")"
"$gc" image assemble-manifest "$workspace/store" "$pin" "$size" "$workspace/staging" --experimental > "$workspace/result.json"
python3 - "$workspace/result.json" <<'PY'
import json,pathlib,sys
s=json.load(open(sys.argv[1]))
assert s["diffIdsVerified"] and s["layerCount"]==2 and s["descriptorSizeVerified"] and s["digestMatches"]
assert not s["sourceProvenanceVerified"] and not s["executionAuthorized"]
p=pathlib.Path(s["directory"])/"etc/data"
assert p.read_bytes()==b"new" and p.stat().st_mode&0o777==0o600
PY
if "$gc" image assemble-manifest "$workspace/store" "$pin" "$((size+1))" "$workspace/staging" --experimental >/dev/null 2>&1; then
  echo "wrong manifest size accepted" >&2;exit 1
fi
test "$(find "$workspace/staging" -mindepth 1 -maxdepth 1 -type d | wc -l)" -eq 1
"$gc" image audit --store "$workspace/store" > "$workspace/audit.json"
python3 - "$workspace/audit.json" <<'PY'
import json,sys
a=json.load(open(sys.argv[1]));assert a["verified"] and a["blobCount"]==4
PY
echo STORE_ASSEMBLY_CLI_PASS
