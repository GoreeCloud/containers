#!/usr/bin/env bash
# Explicit, first-party synthetic workload; never run on a production host.
set -euo pipefail
umask 077
mode="${1:---prepare-only}"
if [[ "$#" -gt 1 || ( "$mode" != --prepare-only && "$mode" != --execute ) ]]; then
    echo 'usage: scripts/oci-smoke.sh [--prepare-only|--execute]' >&2; exit 2
fi
[[ "$(id -u)" != 0 && "$(id -g)" != 0 && "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]]
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
workspace="$(mktemp -d /tmp/gcontainers-smoke.XXXXXXXX)"
echo "Private fixture directory: $workspace"
mkdir -m 700 "$workspace/runtime" "$workspace/bundle"
mkdir -p "$workspace/bundle/rootfs"/{bin,dev/pts,proc}
export XDG_RUNTIME_DIR="$workspace/runtime"
go build -o "$workspace/gcontainers" ./cmd/gcontainers
export PATH=/usr/local/bin:/usr/bin:/bin
gcc -static -O2 -Wall -Wextra -Werror scripts/fixtures/oci-smoke-guest.c -o "$workspace/bundle/rootfs/bin/probe"
python3 - "$workspace/bundle/config.json" <<'PY'
import json,os,sys
caps={name:[] for name in ("bounding","effective","inheritable","permitted","ambient")}
allow=["execve","read","write","close","exit","exit_group","brk","mmap","mprotect","munmap","arch_prctl","set_tid_address","set_robust_list","rseq","prlimit64","getrandom","readlink","readlinkat","openat","newfstatat","fstat","lseek","getpid","getuid","getgid","geteuid","getegid","prctl","capget","rt_sigaction","rt_sigprocmask","rt_sigreturn","clock_nanosleep","nanosleep","clock_gettime","gettid","futex","fcntl","getcwd","statx","access","faccessat2"]
spec={"ociVersion":"1.2.0","process":{"args":["/bin/probe"],"cwd":"/","env":["PATH=/bin"],"user":{"uid":0,"gid":0},"capabilities":caps,"noNewPrivileges":True,"terminal":False},"root":{"path":"rootfs","readonly":True},"mounts":[{"destination":"/proc","type":"proc","source":"proc","options":["ro","nosuid","noexec","nodev"]},{"destination":"/dev","type":"tmpfs","source":"tmpfs","options":["nosuid","noexec","nodev","mode=755","size=65536k"]},{"destination":"/dev/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0620"]}],"linux":{"namespaces":[{"type":n} for n in ["user","pid","mount","network","ipc","uts"]],"uidMappings":[{"containerID":0,"hostID":os.getuid(),"size":1}],"gidMappings":[{"containerID":0,"hostID":os.getgid(),"size":1}],"seccomp":{"defaultAction":"SCMP_ACT_ERRNO","architectures":["SCMP_ARCH_X86_64"],"syscalls":[{"names":allow,"action":"SCMP_ACT_ALLOW"}]}}}
with open(sys.argv[1],"w") as f: json.dump(spec,f)
PY
gc="$workspace/gcontainers"
"$gc" preflight "$workspace/bundle"
if [[ "$mode" == --prepare-only ]]; then
    echo 'PREPARE_ONLY_PASS: no OCI runtime called'
    exit 0
fi
# Only the pinned official binary is accepted for this reproducible fixture.
[[ -x /usr/local/bin/runc ]]
[[ "$(stat -c %u /usr/local/bin/runc)" == 0 ]]
printf '%s  %s\n' 50dcc0b257d724fe07f3852c2dda52340223f038227c7572644eec870741223e /usr/local/bin/runc | sha256sum -c -
"$gc" doctor
id=gc-smoke
state_root="$XDG_RUNTIME_DIR/goreecloud-containers"
# Failures preserve the private workspace and journal. The guest self-expires.
trap 'echo "OCI smoke incomplete: inspect private fixture at $workspace; no force cleanup performed" >&2' ERR
"$gc" oci create --runtime runc --id "$id" --bundle "$workspace/bundle" --experimental
"$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"]=="created"'
"$gc" oci start --runtime runc --id "$id" --experimental
sleep 2
"$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys; s=json.load(sys.stdin); assert s["status"]=="running" and s["pid"]>0; print("running state observed after guest checks")'
"$gc" oci stop --runtime runc --id "$id" --experimental
for i in {1..30}; do
    if "$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"]=="stopped"' 2>/dev/null; then break; fi
    sleep .2
done
"$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"]=="stopped"'
"$gc" oci delete --runtime runc --id "$id" --experimental
/usr/local/bin/runc --root "$state_root" list --format json | python3 -c 'import json,sys; assert not json.load(sys.stdin)'
"$gc" oci pending-summary | python3 -c 'import json,sys; assert json.load(sys.stdin)["pendingCount"]==0'
trap - ERR
echo 'OCI_LIFECYCLE_SMOKE_PASS: first-party guest only; not Stable acceptance'
