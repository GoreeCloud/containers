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
import gzip,hashlib,io,json,pathlib,sys,tarfile
p=pathlib.Path(sys.argv[1])
def layer(entries):
 b=io.BytesIO()
 with tarfile.open(fileobj=b,mode="w",format=tarfile.USTAR_FORMAT) as t:
  for n,v in entries:
   i=tarfile.TarInfo(n);i.size=len(v);i.mode=0o600;t.addfile(i,io.BytesIO(v))
 return b.getvalue()
def pin(b):return "sha256:"+hashlib.sha256(b).hexdigest()
def desc(m,b):return {"mediaType":m,"digest":pin(b),"size":len(b)}
a=layer([("etc/old",b"old"),("etc/sub/lower",b"lower"),("gone/child",b"x"),("keep",b"keep")])
raw=layer([("etc/new",b"new"),("etc/.wh..wh..opq",b""),(".wh.gone",b""),("gone",b"replacement")])
b=gzip.compress(raw,mtime=0)
c=json.dumps({"os":"linux","architecture":"amd64","rootfs":{"type":"layers","diff_ids":[pin(a),pin(raw)]}}).encode()
m=json.dumps({"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":desc("application/vnd.oci.image.config.v1+json",c),"layers":[desc("application/vnd.oci.image.layer.v1.tar",a),desc("application/vnd.oci.image.layer.v1.tar+gzip",b)]}).encode()
for name,data in (("a",a),("b",b),("config",c),("manifest",m)):(p/name).write_bytes(data)
(p/"pin").write_text(pin(m));(p/"size").write_text(str(len(m)))
PY
for b in a b config manifest; do "$gc" image import --store "$d/store" --file "$d/$b" --experimental >/dev/null; done
pin="$(cat "$d/pin")";size="$(cat "$d/size")"
if "$gc" image assemble-manifest "$d/store" "$pin" "$size" "$d/staging" --experimental >/dev/null 2>&1; then echo "default whiteout policy weakened";exit 1;fi
test "$(find "$d/staging" -mindepth 1 -maxdepth 1 | wc -l)" -eq 0
"$gc" image assemble-manifest "$d/store" "$pin" "$size" "$d/staging" --whiteouts --experimental > "$d/result.json"
python3 - "$d/result.json" <<'PY'
import json,pathlib,sys
r=json.load(open(sys.argv[1]));p=pathlib.Path(r["directory"])
assert r["whiteoutsApplied"]==2 and r["diffIdsVerified"] and not r["executionAuthorized"]
assert (p/"etc/new").read_bytes()==b"new" and (p/"gone").read_bytes()==b"replacement" and (p/"keep").read_bytes()==b"keep"
assert not (p/"etc/old").exists() and not (p/"etc/sub").exists() and not (p/"etc/.wh..wh..opq").exists()
PY
echo WHITEOUT_CLI_PASS
