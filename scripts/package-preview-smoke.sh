#!/usr/bin/env bash
# Two isolated builds from committed source, no runtime or external distribution.
set -euo pipefail
umask 077
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
workspace="$(mktemp -d)"
commit="$(git rev-parse HEAD)"
a="$(bash scripts/package-preview.sh "$commit" "$workspace")"
b="$(bash scripts/package-preview.sh "$commit" "$workspace")"
cmp "$a" "$b"
mkdir -m 700 "$workspace/unpacked"
tar -tzf "$a" | diff -u - <(printf '%s\n' BUILD-INFO LICENSE SHA256SUMS gcontainers)
tar -xzf "$a" -C "$workspace/unpacked" --no-same-owner
(cd "$workspace/unpacked"; sha256sum -c SHA256SUMS; test "$(./gcontainers version)" = 0.1.0-dev; grep -Fx 'releaseAuthorized=false' BUILD-INFO; grep -Fx "sourceCommit=$commit" BUILD-INFO; ./gcontainers help >/dev/null)
if bash scripts/package-preview.sh "$(printf '%040d' 0)" "$workspace"; then exit 1; fi
mkdir -m 755 "$workspace/public"
if bash scripts/package-preview.sh "$commit" "$workspace/public"; then exit 1; fi
echo REPRODUCIBLE_PREVIEW_PACKAGE_PASS
