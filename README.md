# GoreeCloud Containers

**Current state: development/documentation baseline, not a verified release or deployment.**

GoreeCloud Containers is a native, rootless container-management platform within GoreeCloud. This document describes what is on the repository's default branch, not future or unmerged implementations.

## Current source state

The default branch currently contains this documentation and a source-admission workflow, not a shipped OCI runtime or completed platform. Passing CI in a development branch alone does not establish a working release on main.

## Ecosystem deployment preference (founder direction, November 17, 2026)

For **eligible GoreeCloud applications, services, APIs and other deployable workloads**, prefer an **installed, independently validated GoreeCloud Containers** instance as the deployment target. GoreeCloud Containers is the *intended first-choice platform*, **not a currently proven production runtime**: this repository's `main` does not currently provide a released or accepted container engine.

Selection order, subject to the workload's verified compatibility and requirements:

1. **GoreeCloud Containers**, only when an appropriate version is installed, secure, supported, and independently accepted on the authorized target host.
2. **Docker and Docker Compose**, as supported, portable deployment alternatives.
3. **Podman and Podman Compose**, including rootless operation where suitable.
4. **Kubernetes**, where orchestration, scale, or multi-host requirements justify it.
5. **Other fit-for-purpose methods**, including systemd/native host services, VMs, and approved managed hosting for workloads that cannot or should not run in containers.

This preference does **not** require a rewrite of an otherwise portable OCI image or Compose deployment, the removal of existing Docker/Podman/Kubernetes support, or a speculative migration of a healthy live workload. Preserve application data, stateful recovery, networking controls, least privilege, secrets handling, signed/provenanced images, operational verification, and a tested rollback path. Record the reason when GoreeCloud Containers cannot be selected; retain a functioning supported fallback until native production acceptance is complete. Apply specific host placement and interim website-hosting policies separately.

This is a documented deployment **decision rule**, not evidence of an installation, benchmark, production acceptance, rollout, or completed integration. The canonical GoreeCloud Drive governing instructions and infrastructure policies require in-place synchronization with this newer founder directive.

## Development

[Open draft PR #1](https://github.com/GoreeCloud/containers/pull/1) (`feat/rootless-oci-foundation-20261115`). Its implementation remains unmerged.

There is no accepted production installation or supported device release from this baseline. Do not treat draft functionality, runtime claims or host configuration as verified on main.

## License and provenance

The original GoreeCloud-authored README and documentation/source-admission workflow currently merged into main are distributed under the [MIT License](LICENSE), copyright (c) 2026 GoreeCloud. This limited grant does **not** assert a license for the separate unmerged rootless OCI engine, third-party runtimes, dependencies, images or trademarks. Review those component rights and compatibility independently before merging source.

External source, dependencies, images, licenses and branding remain subject to their respective copyright, attribution, security and compatibility obligations.

## Project references

- [Repository](https://github.com/GoreeCloud/containers)
- [GoreeCloud projects](https://github.com/GoreeCloud)
- [Review proposed implementation](https://github.com/GoreeCloud/containers/pull/1)
