#!/usr/bin/env bash
set -euo pipefail

# Opt-in only: exercising a real OCI runtime must never be part of default CI.
if [[ "$(printenv GOREECLOUD_CONTAINERS_REAL_CLI_LIFECYCLE || true)" != "1" ]]; then
  echo "Opt in with GOREECLOUD_CONTAINERS_REAL_CLI_LIFECYCLE=1" >&2
  exit 2
fi
if [[ "$(id -u)" == "0" || "$(id -g)" == "0" ]]; then
  echo "Refusing to run under a host-root identity" >&2
  exit 2
fi
for tool in /usr/bin/runc cc jq timeout mktemp stat; do
  command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool" >&2; exit 2; }
done

repo="$(cd "$(dirname "$0")/.." && pwd -P)"
cli="$repo/target/debug/goree"
[[ -x "$cli" ]] || { echo "Build goree first: cargo build -p goree" >&2; exit 2; }

work="$(mktemp -d /tmp/goreecloud-cli-lifecycle.XXXXXX)"
state="$work/state"
bundle="$work/bundle"
container_id="gc-cli-probe-$$"
cleanup() {
  /usr/bin/runc --root "$state" delete --force "$container_id" >/dev/null 2>&1 || true
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
        fputs("Identity/procfs mismatch\n", stderr);
        return 3;
    }
    puts("goree-cli-real-rootless-lifecycle-probe");
    fflush(stdout);
    sleep(2);
    return 0;
}
PROBE
cc -static -s -o "$bundle/rootfs/bin/probe" "$work/probe.c"
"$cli" bundle init-rootless "$bundle" "$(id -u)" "$(id -g)" /bin/probe
jq -e --argjson uid "$(id -u)" --argjson gid "$(id -g)" '
  .root.readonly == true and
  .process.noNewPrivileges == true and
  .linux.uidMappings == [{"containerID":0,"hostID":$uid,"size":1}] and
  .linux.gidMappings == [{"containerID":0,"hostID":$gid,"size":1}] and
  any(.mounts[]; .destination == "/proc" and .type == "proc" and .source == "proc")
' "$bundle/config.json" >/dev/null
[[ "$(stat -c %a "$bundle/config.json")" == "600" ]] || exit 1

# The executable is a disposable test-only wrapper; all runtime state is
# confined to a fresh directory, never runc's normal per-user state location.
cat >"$work/runc-isolated" <<'WRAPPER'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "create" ]]; then
  # runc-created init processes can hold inherited pipes open until start.
  # Redirect create's descriptors to local files so the parent CLI can exit.
  set +e
  /usr/bin/runc --root "$GOREECLOUD_CLI_LIFECYCLE_STATE" "$@" \
    >"$GOREECLOUD_CLI_LIFECYCLE_STATE/create.stdout" \
    2>"$GOREECLOUD_CLI_LIFECYCLE_STATE/create.stderr"
  result=$?
  set -e
  cat "$GOREECLOUD_CLI_LIFECYCLE_STATE/create.stdout"
  cat "$GOREECLOUD_CLI_LIFECYCLE_STATE/create.stderr" >&2
  exit "$result"
fi
exec /usr/bin/runc --root "$GOREECLOUD_CLI_LIFECYCLE_STATE" "$@"
WRAPPER
chmod 700 "$work/runc-isolated"
export GOREECLOUD_CLI_LIFECYCLE_STATE="$state"
runtime="$work/runc-isolated"

timeout --kill-after=3s 25s "$cli" runtime create runc "$runtime" "$container_id" "$bundle"
"$cli" runtime state runc "$runtime" "$container_id" |
  jq -e --arg id "$container_id" '.id == $id and .status == "created"' >/dev/null
timeout --kill-after=3s 25s "$cli" runtime start runc "$runtime" "$container_id"
sleep 3
if ! grep -Fq "goree-cli-real-rootless-lifecycle-probe" "$state/create.stdout"; then
  echo "Expected workload output was not observed after runtime start" >&2
  exit 1
fi
"$cli" runtime state runc "$runtime" "$container_id" |
  jq -e --arg id "$container_id" '.id == $id and (.status == "stopped" or .status == "running")' >/dev/null
timeout --kill-after=3s 25s "$cli" runtime delete runc "$runtime" "$container_id"
if /usr/bin/runc --root "$state" state "$container_id" >/dev/null 2>&1; then
  echo "Failed to delete disposable OCI test state" >&2
  exit 1
fi
echo "GoreeCloud CLI real-runc rootless create/state/start/state/delete passed"
