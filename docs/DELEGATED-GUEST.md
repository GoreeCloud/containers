# Explicit delegated non-root guest preview

The original caller-root profile remains unchanged. A separate x86-64 Linux runc preview accepts exactly two single-ID UID mappings and two single-ID GID mappings: guest root maps to the nonroot invoking host user/group, guest UID/GID 1000 maps to one existing subordinate UID/GID from the root-managed local allocation files. Reject unallocated/overlapping identities, actual account/group collision, non-files subid backends, unsafe helper ownership/mode, other mapping shapes and additional privilege. No host allocations, account changes, helper installation or direct mapping helper invocation by gcontainers.

A private mode-0700 bundle and parent keep the guest-readable rootfs off the host's public path. Only caller-owned regular files, single links, safe bounded trees and same mount are admitted. Guest directories mode 0755, payload files 0444/0555; config 0400/0600. Rootfs is readonly; no capabilities, no new privileges, strict private namespaces and default-deny seccomp remain mandatory. Only static native x86-64 ELF entrypoint for this profile. The separate image preparation path keeps only the exact pinned executable executable, with other payload inert. Snapshot mode normalizes a fresh copy and domain-separates its digest. No rootfs immutability, source provenance or full filesystem race guarantee.

Read-only preflight:

```sh
gcontainers preflight-delegated ABS_BUNDLE
```

Prepare a fresh private snapshot or a separately pinned manifest image:

```sh
gcontainers snapshot-delegated ABS_BUNDLE PRIVATE_PARENT --experimental
gcontainers image prepare-bundle EXISTING_STORE MANIFEST_SHA SIZE CONFIG_FILE CONFIG_SHA ENTRYPOINT_SHA PRIVATE_PARENT --whiteouts --delegated-guest --experimental
```

The whiteout flag remains optional and independent. Preparation is not authorization. Explicit individually reviewed local nonproduction runtime preview only:

```sh
gcontainers oci create --runtime runc --id NAME --bundle ABS_BUNDLE --delegated-guest --experimental
gcontainers oci start --runtime runc --id NAME --experimental
gcontainers oci stop --runtime runc --id NAME --experimental
gcontainers oci delete --runtime runc --id NAME --experimental
```

Original create without the new flag refuses delegated mapping. Existing cooperative journal and cgroup post-create evidence remain enforced. Runtime state does not identify an authenticated application or attest future execution. No automatic native Stack deployment, network attachment, mounts, restart, identity integration, MIPS authorization or production acceptance.

## Acceptance

November 26, 2026: laptop Go 1.27.4, verified runc 1.5.3, existing Ubuntu uidmap helpers and local subordinate allocation. The first-party static guest asserts UID/GID 1000, NNP, seccomp, zero capabilities, read-only root mount, socket denial, bounded self-expiry. Actual host status confirms mapped subordinate identities and zero capabilities; cgroup bounds read back by engine. Both isolated snapshot and pinned tar/gzip whiteout image lifecycle completed, pending census empty. Default caller-root lifecycle still passes. Helper allocation values are not emitted in reports. Test scripts require already allocated ranges, do not modify /etc, and preserve uncertain evidence. Hosted CI remains synthetic/unprivileged and does not assume delegated runtime availability.

```sh
bash scripts/delegated-guest-smoke.sh --prepare-only
bash scripts/delegated-guest-smoke.sh --execute
bash scripts/prepared-bundle-smoke.sh --execute --whiteouts --delegated-guest
```

### Live read-only delegated evidence

Use gcontainers delegated-observe runc ID ABS_BUNDLE to compare a separately reviewed bundle's subordinate mapping and limits with actual running process /proc status under the cooperative ID lock. Verify all four host UID/GID values, nested PID1, zero capability sets, NNP, seccomp filter and configured cgroup limits. Repeat runtime state and process status before reporting. No PIDs or mapping numbers emitted, no mutation, no proof that the specified bundle owns the workload or that health/namespace containment is certified.

### Isolated local I/O acceptance fixture

On the authorized laptop the existing delegated profile ran a separately reviewed first-party static test variant, explicitly allowing the minimum additional socket and scratch-file syscalls in its test configuration. The guest confirmed only loopback was present in its private network namespace, opened a TCP listener on IPv4 loopback with a kernel-selected port, connected only to that listener, transferred and compared synthetic bytes, then created/read/deleted synthetic bytes in the already supported bounded private /tmp tmpfs. The root mount stayed read-only; NNP, seccomp, zero capabilities, host subordinate UID/GID and cgroup evidence passed. Graceful stop/delete and empty pending journal verified. No host socket, route, forwarding, host security or account change. The original socket-denial fixture remains the default and unchanged.

```sh
bash scripts/delegated-guest-smoke.sh --execute --local-io
```

This is an actual local I/O fixture, not an inter-container network, gateway, host port publication, network policy controller or persistent volume attachment. The extra syscall list is specific to the reviewed synthetic config, not an authorization for arbitrary networked images. The native Stack controller still refuses its socket syscalls; general networking and durable storage acceptance remain open.
