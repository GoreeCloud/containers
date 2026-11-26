#!/usr/bin/env bash
# Isolated native UI startup and real empty private Unix bridge.
# Does not start any container, contact production, or use the active display.
set -euo pipefail
umask 077
[[ "$(id -u)" != 0 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
repo="$PWD"
d="$(mktemp -d)"
pid=""
cleanup(){
 if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true;wait "$pid" 2>/dev/null || true; fi
 rm -rf -- "$d"
}
trap cleanup EXIT
mkdir -m 700 "$d/xdg" "$d/xdg/goreecloud-containers" "$d/xdg/goreecloud-containers/locks" "$d/xdg/goreecloud-containers/journal" "$d/home" "$d/config" "$d/cache" "$d/data"
go build -o "$d/gcontainers" ./cmd/gcontainers
pin="sha256:$(sha256sum "$d/gcontainers" | cut -d' ' -f1)"
export PATH=/usr/local/bin:$PATH
XDG_RUNTIME_DIR="$d/xdg" "$d/gcontainers" serve-local "$d/xdg/api.sock" --runtime runc --experimental >"$d/server.log" 2>&1 & pid=$!
for i in {1..100}; do [[ -S "$d/xdg/api.sock" ]] && break; sleep 0.05; done
test -S "$d/xdg/api.sock"
"$d/gcontainers" local-query "$d/xdg/api.sock" runtime-summary | python3 -c 'import json,sys;v=json.load(sys.stdin);assert v["total"]==0 and v["readOnly"]'
cd "$repo/desktop/dashboard"
GC_TEST_CLI="$d/gcontainers" GC_TEST_CLI_PIN="$pin" GC_TEST_SOCKET="$d/xdg/api.sock" flutter test --reporter expanded test/live_summary_test.dart
exe="$PWD/build/linux/x64/debug/bundle/goreecloud_containers"
test -x "$exe"
export GC_NATIVE_TEST_EXE="$exe" GC_NATIVE_TEST_CLI="$d/gcontainers" GC_NATIVE_TEST_PIN="$pin" GC_NATIVE_TEST_SOCKET="$d/xdg/api.sock" GC_NATIVE_TEST_ROOT="$d"
xvfb-run -a --server-args="-screen 0 1280x800x24" bash -c '
 set -euo pipefail
 export HOME="$GC_NATIVE_TEST_ROOT/home" XDG_RUNTIME_DIR="$GC_NATIVE_TEST_ROOT/xdg" XDG_CONFIG_HOME="$GC_NATIVE_TEST_ROOT/config" XDG_CACHE_HOME="$GC_NATIVE_TEST_ROOT/cache" XDG_DATA_HOME="$GC_NATIVE_TEST_ROOT/data" LIBGL_ALWAYS_SOFTWARE=1
 "$GC_NATIVE_TEST_EXE" "$GC_NATIVE_TEST_CLI" "$GC_NATIVE_TEST_PIN" "$GC_NATIVE_TEST_SOCKET" >"$GC_NATIVE_TEST_ROOT/window.log" 2>&1 & app=$!
 trap '\''kill -TERM "$app" 2>/dev/null || true;wait "$app" 2>/dev/null || true'\'' EXIT
 for i in {1..100}; do
  if xwininfo -root -tree | grep -Fq "GoreeCloud Containers"; then break;fi
  kill -0 "$app";sleep 0.1
 done
 xwininfo -name "GoreeCloud Containers" | grep -E "Width: 960|Height: 680" > "$GC_NATIVE_TEST_ROOT/geometry"
 test "$(wc -l < "$GC_NATIVE_TEST_ROOT/geometry")" -eq 2
 kill -0 "$app"
 echo NATIVE_DASHBOARD_WINDOW_PASS
'
echo DESKTOP_DASHBOARD_BRIDGE_PASS
