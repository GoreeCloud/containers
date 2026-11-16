# GoreeCloud Containers

**An experimental, GoreeCloud-owned Linux container engine and management platform.**

GoreeCloud Containers is being built as the eventual default containerization solution for the GoreeCloud ecosystem, with a design goal of replacing Docker and Podman management while retaining portable OCI images and Compose-compatible deployment definitions.

> **Status: 0.1.0-dev / experimental foundation. Not production-ready.** No Docker/Podman replacement, image pull/build, daemon, Compose execution, networking, volumes, UI, remote control, or cluster orchestration is claimed. Do not migrate production containers.

## Implemented in this development branch

- `gcontainers doctor`: a read-only report of local Linux and OCI runtime prerequisites.
- `gcontainers validate <bundle-directory>`: fail-closed parsing of a restricted rootless OCI bundle policy. Includes file-size and JSON nesting limits, duplicate-key rejection, and restrictions on host namespaces, root filesystem, capabilities, devices, hooks, writable/bind mounts, and seccomp defaults.
- `gcontainers oci create|start|state|stop|delete`: a GoreeCloud-owned lifecycle adapter that invokes an external, replaceable OCI low-level runtime (`crun` or `runc`) directly, without Docker, Podman, containerd, a shell, or a background daemon. Mutations require `--experimental`; runtime state is stored in a private per-user directory and deletion is never forced.
- Standard-library-only Go code, initial unit tests and Linux CI.

The execution adapter is **not** yet independently security audited or exercised against real kernel isolation in this change. OCI runtimes remain required for execution; this is not a from-scratch kernel runtime. Validation reduces obvious hazards, but cannot certify an arbitrary OCI bundle as safe.

## Build and try the read-only commands

Requires Linux and Go 1.27 or later.

```sh
go test ./...
go build -o gcontainers ./cmd/gcontainers
./gcontainers doctor
./gcontainers validate /path/to/oci-bundle
```

Running an experimental rootless bundle also requires a maintained `crun` or `runc` installation, a private `XDG_RUNTIME_DIR`, a suitable Linux kernel, correctly mapped UID/GID ranges, rootfs, and an OCI spec satisfying the preview policy. Execution is not recommended on production hosts.

```sh
./gcontainers oci create --id example --bundle /path/to/oci-bundle --experimental
./gcontainers oci start --id example --experimental
./gcontainers oci state --id example
./gcontainers oci stop --id example --experimental
./gcontainers oci delete --id example --experimental
```

## Architecture, safety, and roadmap

- [Architecture](docs/architecture.md)
- [Security boundaries and limitations](docs/security.md)
- [Milestones and acceptance criteria](docs/roadmap.md)
- [Docker and Podman migration strategy](docs/migration.md)

### Integration boundaries

- **GoreeCloud Mesh:** service discovery, policy-aware APIs, event delivery, and integration (future, not implemented).
- **GoreeCloud Identity:** authenticate and authorize remote management (future; no remote endpoint currently exists).
- **GoreeShield:** consent, data minimization, retention, and privacy controls (future adapter).
- **Wardveil:** container security policy, image scanning, detection and response (future adapter).
- **GoreeCloud UI:** the sole design authority for any future graphical management client.

## Licensing

The original GoreeCloud source in this repository is offered under the [MIT License](LICENSE). No third-party runtime source is vendored. OCI specifications and `crun`/`runc` are independent upstream projects with their own licenses.
