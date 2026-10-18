#!/usr/bin/env bash
set -euo pipefail

if [[ "$(printenv GOREECLOUD_CONTAINERS_REAL_ROOTLESS_SMOKE || true)" != "1" ]]; then
  echo "Opt in with GOREECLOUD_CONTAINERS_REAL_ROOTLESS_SMOKE=1" >&2
  exit 2
fi
if [[ "$(id -u)" == "0" || "$(id -g)" == "0" ]]; then
  echo "Refusing host-root execution" >&2
  exit 2
fi
for tool in runc cc timeout jq stat mktemp; do
  command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool" >&2; exit 2; }
done

repo="$(cd "$(dirname "$0")/.." && pwd -P)"
cli="$repo/target/debug/goree"
[[ -x "$cli" ]] || { echo "Build goree first: cargo build -p goree" >&2; exit 2; }

work="$(mktemp -d /tmp/goreecloud-rootless-acceptance.XXXXXX)"
state="$work/runtime-state"
bundle="$work/bundle"
container_id="gc-rootless-probe-$$"
cleanup() {
  runc --root "$state" delete --force "$container_id" >/dev/null 2>&1 || true
  [[ -d "$work" ]] && rm -rf -- "$work"
}
trap cleanup EXIT

mkdir -p "$state" "$bundle/rootfs/bin" "$bundle/rootfs/proc"
cat >"$work/probe.c" <<'PROBE'
#include <linux/magic.h>
#include <stdio.h>
#include <sys/vfs.h>
#include <unistd.h>
int main(void) {
    struct statfs info;
    if (getuid() != 0 || getgid() != 0 ||
        statfs("/proc", &info) != 0 || info.f_type != PROC_SUPER_MAGIC) {
        fputs("Rootless identity or procfs verification failed\n", stderr);
        return 3;
    }
    puts("rootless-container-user-and-procfs-verified");
    return 0;
}
PROBE
cc -static -s -o "$bundle/rootfs/bin/probe" "$work/probe.c"
"$cli" bundle init-rootless "$bundle" "$(id -u)" "$(id -g)" /bin/probe

jq -e --argjson uid "$(id -u)" --argjson gid "$(id -g)" '
  .root.readonly == true and .root.path == "rootfs" and
  .process.noNewPrivileges == true and
  (.linux.uidMappings == [{"containerID":0,"hostID":$uid,"size":1}]) and
  (.linux.gidMappings == [{"containerID":0,"hostID":$gid,"size":1}]) and
  any(.mounts[]; .destination == "/proc" and .type == "proc" and .source == "proc")
' "$bundle/config.json" >/dev/null
[[ "$(stat -c %a "$bundle/config.json")" == "600" ]] ||
  { echo "Expected private OCI config mode 0600" >&2; exit 1; }

timeout --kill-after=3s 20s runc --root "$state" run --bundle "$bundle" "$container_id"
echo "Disposable rootless runc acceptance smoke passed"
