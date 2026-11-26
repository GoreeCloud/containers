#!/usr/bin/env bash
# Offline synthetic OCI layout; no registry, credentials or runtime invocation.
set -euo pipefail
umask 077
[[ "$(id -u)" != 0 && "$#" == 0 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
work="$(mktemp -d /tmp/gcontainers-layout.XXXXXXXX)"
echo "Private layout fixture: $work"
mkdir -p "$work/layout/blobs/sha256" "$work/staged"
go build -o "$work/gcontainers" ./cmd/gcontainers
gc="$work/gcontainers"
python3 - "$work" <<'PY'
import gzip,hashlib,io,json,pathlib,sys,tarfile
p=pathlib.Path(sys.argv[1]);layout=p/"layout";blobs=layout/"blobs/sha256"
def pin(b):return "sha256:"+hashlib.sha256(b).hexdigest()
def obj(x):return json.dumps(x,separators=(",",":")).encode()
def put(media,b):
 d=pin(b);(blobs/d[7:]).write_bytes(b)
 return dict(mediaType=media,digest=d,size=len(b))
def layer(entries):
 out=io.BytesIO()
 with tarfile.open(fileobj=out,mode="w",format=tarfile.USTAR_FORMAT) as t:
  for name,data in entries:
   i=tarfile.TarInfo(name);i.size=len(data);t.addfile(i,io.BytesIO(data))
 return out.getvalue()
a=layer([("etc/old",b"old")]);b=layer([("etc/.wh.old",b""),("etc/new",b"new")])
l=[put("application/vnd.oci.image.layer.v1.tar",a),put("application/vnd.oci.image.layer.v1.tar+gzip",gzip.compress(b,mtime=0))]
c=put("application/vnd.oci.image.config.v1+json",obj(dict(os="linux",architecture="amd64",rootfs=dict(type="layers",diff_ids=[pin(a),pin(b)]))))
m=put("application/vnd.oci.image.manifest.v1+json",obj(dict(schemaVersion=2,mediaType="application/vnd.oci.image.manifest.v1+json",config=c,layers=l)))
index=obj(dict(schemaVersion=2,mediaType="application/vnd.oci.image.index.v1+json",manifests=[m]))
(layout/"index.json").write_bytes(index);(layout/"oci-layout").write_bytes(obj(dict(imageLayoutVersion="1.0.0")))
(p/"pins").write_text("\n".join((pin(index),m["digest"],str(m["size"])))+"\n")
(p/"seed").write_bytes(b"synthetic unrelated seed")
PY
mapfile -t pins < "$work/pins"
"$gc" image import --store "$work/store" --file "$work/seed" --experimental >/dev/null
if "$gc" image import-layout "$work/store" "$work/layout" "sha256:$(printf '%064d' 0)" "${pins[1]}" "${pins[2]}" "$work/staged" --experimental;then exit 1;fi
test -z "$(find "$work/staged" -mindepth 1 -print -quit)"
"$gc" image import-layout "$work/store" "$work/layout" "${pins[0]}" "${pins[1]}" "${pins[2]}" "$work/staged" --whiteouts --experimental > "$work/first.json"
"$gc" image import-layout "$work/store" "$work/layout" "${pins[0]}" "${pins[1]}" "${pins[2]}" "$work/staged" --whiteouts --experimental > "$work/second.json"
python3 - "$work" <<'PY'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1])
for name,counts in (("first",(4,0)),("second",(0,4))):
 r=json.loads((p/(name+".json")).read_text())
 assert (r["importedBlobs"],r["reusedBlobs"])==counts and r["diffIdsVerified"] and not r["executionAuthorized"] and r["whiteoutsApplied"]==1
 root=pathlib.Path(r["directory"]);assert (root/"etc/new").read_bytes()==b"new" and not (root/"etc/old").exists()
PY
"$gc" image audit --store "$work/store" >/dev/null
echo OFFLINE_LAYOUT_IMPORT_PASS

mkdir -m 700 "$work/exports" "$work/roundtrip"
if "$gc" image export-layout "$work/store" "${pins[1]}" "${pins[2]}" "$work/exports" --experimental;then exit 1;fi
"$gc" image export-layout "$work/store" "${pins[1]}" "${pins[2]}" "$work/exports" --whiteouts --experimental > "$work/export.json"
mapfile -t exported < <(python3 - "$work/export.json" <<'PYEXPORT'
import json,sys
r=json.load(open(sys.argv[1]));assert r["verifiedBlobs"]==4 and r["diffIdsVerified"] and r["archiveSubsetVerified"] and not r["sourceProvenanceVerified"] and not r["executionAuthorized"]
print(r["directory"]);print(r["indexDigest"]);print(r["manifestDigest"]);print(r["manifestSize"])
PYEXPORT
)
"$gc" image verify "${exported[0]}" >/dev/null
"$gc" image import --store "$work/second-store" --file "$work/seed" --experimental >/dev/null
"$gc" image import-layout "$work/second-store" "${exported[0]}" "${exported[1]}" "${exported[2]}" "${exported[3]}" "$work/roundtrip" --whiteouts --experimental > "$work/roundtrip.json"
python3 - "$work" <<'PYROUND'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1]);x=json.load(open(p/"roundtrip.json"));assert x["importedBlobs"]==4 and x["diffIdsVerified"] and x["whiteoutsApplied"]==1
root=pathlib.Path(x["directory"]);assert (root/"etc/new").read_bytes()==b"new" and not (root/"etc/old").exists()
PYROUND
"$gc" image audit --store "$work/second-store" >/dev/null
echo OFFLINE_LAYOUT_ROUNDTRIP_PASS
