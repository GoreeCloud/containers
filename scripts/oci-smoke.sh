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
# runc 1.5.3/libpathrs needs fstatfs to validate procfs and getdents64
# to close inherited file descriptors before exec. No broad allow or
# privileged syscall is added; the guest still asserts socket denial.
allow=["execve","read","write","close","exit","exit_group","brk","mmap","mprotect","munmap","arch_prctl","set_tid_address","set_robust_list","rseq","prlimit64","getrandom","readlink","readlinkat","openat","newfstatat","fstat","fstatfs","getdents64","lseek","getpid","getuid","getgid","geteuid","getegid","prctl","capget","rt_sigaction","rt_sigprocmask","rt_sigreturn","clock_nanosleep","nanosleep","clock_gettime","gettid","futex","fcntl","getcwd","statx","access","faccessat2"]
spec={"ociVersion":"1.2.0","process":{"args":["/bin/probe"],"cwd":"/","env":["PATH=/bin"],"user":{"uid":0,"gid":0},"capabilities":caps,"noNewPrivileges":True,"terminal":False},"root":{"path":"rootfs","readonly":True},"mounts":[{"destination":"/proc","type":"proc","source":"proc","options":["ro","nosuid","noexec","nodev"]},{"destination":"/dev","type":"tmpfs","source":"tmpfs","options":["nosuid","noexec","nodev","mode=755","size=65536k"]},{"destination":"/dev/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0620"]}],"linux":{"namespaces":[{"type":n} for n in ["user","pid","mount","network","ipc","uts"]],"uidMappings":[{"containerID":0,"hostID":os.getuid(),"size":1}],"gidMappings":[{"containerID":0,"hostID":os.getgid(),"size":1}],"seccomp":{"defaultAction":"SCMP_ACT_ERRNO","architectures":["SCMP_ARCH_X86_64"],"syscalls":[{"names":allow,"action":"SCMP_ACT_ALLOW"}]}}}
if os.environ.get("GC_SMOKE_RESOURCES") == "1":
    spec["linux"]["resources"]={"cpu":{"quota":50000,"period":100000},"memory":{"limit":67108864},"pids":{"limit":32}}
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
id="gc-smoke-$(printf %s "${workspace##*.}" | tr A-Z a-z)"
state_root="$XDG_RUNTIME_DIR/goreecloud-containers"
# Failures preserve the private workspace and journal. The guest self-expires.
trap 'echo "OCI smoke incomplete: inspect private fixture at $workspace; no force cleanup performed" >&2' ERR
"$gc" oci create --runtime runc --id "$id" --bundle "$workspace/bundle" --experimental
"$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"]=="created"'
"$gc" oci start --runtime runc --id "$id" --experimental
sleep 2
# Check the actual host-observed process, not just the runtime's success code.
"$gc" oci state --runtime runc --id "$id" > "$workspace/running.json"
python3 - "$workspace/running.json" <<'PYHOST'
import json,os,sys
s=json.load(open(sys.argv[1]))
assert s["status"]=="running" and s["pid"]>0
p=s["pid"]
# Compare identities, not just presence, without printing namespace inodes.
for ns in ("user","mnt","pid","net","ipc","uts"):
    host=os.stat(f"/proc/self/ns/{ns}")
    guest=os.stat(f"/proc/{p}/ns/{ns}")
    assert (host.st_dev,host.st_ino)!=(guest.st_dev,guest.st_ino), f"shared {ns} namespace"
print("SIX_NAMESPACE_IDENTITIES_PASS")
with open(f"/proc/{p}/status") as f:
    status=dict(line.rstrip().split(":",1) for line in f if ":" in line)
assert status["Name"].strip()=="probe", "runtime init has not exec'd the test guest"
assert status["Uid"].split()==[str(os.getuid())]*4
assert status["Gid"].split()==[str(os.getgid())]*4
assert status["NoNewPrivs"].strip()=="1"
assert status["Seccomp"].strip()=="2"
for name in ("CapInh","CapPrm","CapEff","CapBnd","CapAmb"):
    assert int(status[name].strip(),16)==0
assert status["NSpid"].split()[-1]=="1"
print("host-observed guest PID namespace, UID/GID, seccomp, NNP and capabilities verified")
if os.environ.get("GC_SMOKE_RESOURCES") == "1":
    from pathlib import Path
    rows=Path(f"/proc/{p}/cgroup").read_text().splitlines()
    paths=[line[3:] for line in rows if line.startswith("0::")]
    assert len(paths)==1 and paths[0].startswith("/")
    cg=Path("/sys/fs/cgroup").joinpath(paths[0].lstrip("/")).resolve(strict=True)
    assert cg.is_relative_to("/sys/fs/cgroup") and cg.name==s["id"]
    assert (cg/"cpu.max").read_text().split()==["50000","100000"]
    assert (cg/"memory.max").read_text().strip()=="67108864"
    assert (cg/"pids.max").read_text().strip()=="32"
    assert str(p) in (cg/"cgroup.procs").read_text().split()
    print("CGROUP_LIMITS_READBACK_PASS: CPU=0.5 memory=64MiB pids=32")
print("running state observed after guest checks")
PYHOST
"$gc" oci stop --runtime runc --id "$id" --experimental
for i in {1..30}; do
    if "$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"]=="stopped"' 2>/dev/null; then break; fi
    sleep .2
done
"$gc" oci state --runtime runc --id "$id" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"]=="stopped"'
"$gc" oci delete --runtime runc --id "$id" --experimental
# State root also contains GoreeCloud journal/lock directories, which runc
# list may mistake for container IDs. Verify this exact isolated ID is gone.
test ! -e "$state_root/$id"
test ! -L "$state_root/$id"
if /usr/local/bin/runc --root "$state_root" state "$id" > "$workspace/deleted-state.json" 2>/dev/null; then
    echo "deleted container still visible" >&2; exit 1
fi
"$gc" oci pending-summary | python3 -c 'import json,sys; assert json.load(sys.stdin)["pendingCount"]==0'
trap - ERR
echo 'OCI_LIFECYCLE_SMOKE_PASS: first-party guest only; not Stable acceptance'
