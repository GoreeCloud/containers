#!/usr/bin/env bash
# Local development artifact only. No install, upload, tag or signature.
set -euo pipefail
umask 077
[[ $# == 2 && "$1" =~ ^[a-f0-9]{40}$ && "$(id -u)" != 0 && "$(uname -s)" == Linux ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
commit="$1"
parent="$2"
[[ "$parent" == /* && "$(realpath -e -- "$parent")" == "$parent" && -d "$parent" && ! -L "$parent" && "$(stat -c %u "$parent")" == "$(id -u)" && "$(stat -c %a "$parent")" == 700 ]]
[[ "$(git rev-parse --verify "$commit^{commit}")" == "$commit" ]]
[[ "$(go version)" == 'go version go1.27.4 linux/amd64' ]]
work="$(mktemp -d "$parent/package-preview.XXXXXXXX")"
# Keep failed artifacts for review; no deletion of an operator-selected path.
mkdir -m 700 "$work/source" "$work/payload"
git archive --format=tar "$commit" -- go.mod cmd internal LICENSE | tar -xf - -C "$work/source"
(
 cd "$work/source"
 export GOENV=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS='' CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1
 go build -trimpath -buildvcs=false -o "$work/payload/gcontainers" ./cmd/gcontainers
)
chmod 0755 "$work/payload/gcontainers"
cp -- "$work/source/LICENSE" "$work/payload/LICENSE"
chmod 0644 "$work/payload/LICENSE"
version="$("$work/payload/gcontainers" version)"
[[ "$version" == 0.1.0-dev ]]
printf 'sourceCommit=%s\nversion=%s\ntarget=linux/amd64\ntoolchain=go1.27.4\nreleaseAuthorized=false\nprovenanceVerified=false\n' "$commit" "$version" > "$work/payload/BUILD-INFO"
chmod 0644 "$work/payload/BUILD-INFO"
(cd "$work/payload"; sha256sum BUILD-INFO LICENSE gcontainers > SHA256SUMS; chmod 0644 SHA256SUMS)
archive="$work/gcontainers-preview-linux-amd64.tar.gz"
tar --format=ustar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C "$work/payload" -cf - BUILD-INFO LICENSE SHA256SUMS gcontainers | gzip -n > "$archive"
(cd "$work"; sha256sum gcontainers-preview-linux-amd64.tar.gz > archive.sha256)
printf '%s\n' "$archive"
