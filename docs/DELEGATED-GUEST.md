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
