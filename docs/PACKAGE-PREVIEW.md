# Local CLI package qualification

Use only a reviewed commit and a clean absolute caller-owned mode-0700 output parent on an authorized Linux amd64 development host. No root, network module resolution, runtime invocation or installation is required.

```sh
bash scripts/package-preview.sh COMMIT_SHA PRIVATE_PARENT
bash scripts/package-preview-smoke.sh
```

The command creates a fresh private directory and packages a static development CLI, exact source LICENSE, BUILD-INFO and payload SHA256SUMS; separate archive.sha256 is alongside the archive. The smoke test compares independently built archive bytes, inspects the fixed member list, unpacks privately, verifies payload hashes and invokes only version/help. Bad commit and public output directory are refused. Failed private artifacts are retained for review. These hashes detect corruption, not malicious replacement or signer identity. Build metadata deliberately denies verified provenance and release authorization. Reproducibility is scoped to the tested toolchain and architecture; signing, verified copyright/redistribution rights, package-manager installation, upgrade/rollback, desktop packages and cross-platform reproducibility remain open. Do not upload or distribute preview artifacts.
