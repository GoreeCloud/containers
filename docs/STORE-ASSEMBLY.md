# Assemble a pinned OCI manifest from the local content store

`gcontainers image assemble-manifest EXISTING_STORE SHA256 SIZE PRIVATE_PARENT --experimental` reads a manifest already imported into the private content store. It verifies its exact SHA-256/size, config digest/size, Linux amd64/arm64 metadata, and the ordered rootfs diff_ids. Each layer is opened as a bounded private regular file and staged while checking both the compressed descriptor and uncompressed tar digest, including padding and gzip CRC. A mismatch discards the new staging tree. Configuration env, command and annotations are not returned or executed. The existing inert staging restrictions still apply: no whiteouts, links, special files, type replacements or executable permissions. This is not a full OCI rootfs, publisher signature, immutable snapshot or launch authorization.

## Bounded registry-to-staging pipeline

`gcontainers image pull-stage EXISTING_STORE HTTPS_ORIGIN REPOSITORY SHA256 SIZE PRIVATE_PARENT --experimental` uses the same private store and exact manifest pin. It verifies cached objects before reuse, fetches and validates config before downloading layer blobs, deduplicates descriptors, and calls the same offline assembly with diff IDs. Optional pre-issued bearer-file authentication; no redirects, automatic challenge exchange, tag resolution, publisher verification or execution; all network requests use the explicitly selected HTTPS origin. Verified CAS blobs can remain after partial failure; the staging tree is discarded, and there is no atomic image/tag registration. Local TLS end-to-end test verifies download, cache reuse without network, corruption rejection and no partial staging.

The repeatable first-party `bash scripts/store-assembly-smoke.sh` fixture imports two layers (tar and gzip), config and manifest, assembles the ordered data through the actual CLI, checks the replaced file/mode, rejects wrong descriptor size without an extra tree and audits all four stored blobs. CI runs this fixture without networking or launching a guest.

## Explicit registry bearer authentication

Append --bearer-file PRIVATE_TOKEN_FILE before --experimental. A pre-issued least-privilege read token for the exact approved HTTPS origin and repository is required; token file owner-only 0400/0600, private parent, no symlinks or hardlinks, bounded single-line bearer grammar. The transport sends it only on GET of pinned manifest/blob paths for that exact origin/repository; no proxy, redirects, token discovery, challenge exchange, refresh, logging or persistence by the engine. The token file remains operator-controlled; no credential value in argv or output. Cached object reuse does not establish remote authentication. Synthetic TLS registry, redirect refusal, out-of-scope requests and private-file regression tests passed; no actual private registry credential was used. Does not establish GoreeCloud Identity integration or publisher provenance.

## Explicit offline whiteout subset

Append --whiteouts before --experimental to image assemble-manifest. Every seekable pinned layer is parsed and digest/DiffID checked before applying zero-length regular whiteout markers to previously staged lower-layer entries. Named deletions and opaque directories (including root) remove only tracked entries, leaf-first, before same-layer files are added, independent of tar member order. Reparse and verify exact bytes in the second pass; any failure discards the new inert tree. No untracked recursive removal, links, full OCI extraction, image execution or source trust. Normal assembly, pull-stage and prepare-bundle reject whiteouts by default; each accepts them only with the explicit --whiteouts option. Tests cover same-layer replacement, named and opaque markers, malformed paths, nonempty markers, invalid diff IDs and cleanup. Same-UID races and non-atomic staging remain outside the guarantee.

The synthetic whiteout CLI fixture scripts/whiteout-smoke.sh passed and is included in protected CI. It verifies gzip and tar lower-layer removal, same-layer replacement, and unchanged default whiteout rejection.

## Pinned local platform selection

gcontainers image select-index EXISTING_STORE INDEX_SHA SIZE amd64|arm64 verifies a single-level OCI index and selects exactly one Linux baseline platform, then verifies that manifest and its config bytes and platform/diff-ID metadata. Refuses duplicate digest, ambiguity, wrong config platform, unsupported constraints or nested index, missing bytes and wrong pin. Emits only pinned descriptors; layer contents, provenance and execution are not verified. Use the returned manifest digest and size with existing assemble-manifest or prepare-bundle. No registry access or tag lookup.

The synthetic CLI fixture scripts/index-selection-smoke.sh validates local amd64 and arm64 selection and subsequent exact manifest assembly, with no network or runtime execution. Added to hosted CI.

## Explicit registry-to-whiteout assembly

Append --whiteouts before --experimental in pull-stage (after optional --bearer-file PRIVATE_TOKEN_FILE). Reuses the pinned HTTPS retrieval, store audit and exact manifest/config/layer verification, then applies the existing double-pass inert whiteout policy. Default still refuses whiteouts. Synthetic TLS registry tests verify anonymous and scoped bearer retrieval, tar/gzip replacement, default refusal, cache reuse, and no token in the report. No tag resolution, external credential exchange, general rootfs semantics or execution.

## Offline pinned OCI layout import

```sh
gcontainers image import-layout EXISTING_STORE ABS_LAYOUT INDEX_SHA MANIFEST_SHA SIZE PRIVATE_PARENT [--whiteouts] --experimental
```

Explicitly select one direct manifest from an independently pinned index.json. Requires a valid OCI 1.0.0 layout marker, unique direct descriptors, exact manifest size/digest, exact configuration, supported Linux architecture and bounded tar/gzip layers. Reads through a checked source directory anchor, independently verifies every imported byte with existing CAS admission (including reused objects), and reuses ordered inert assembly for decoded DiffID verification. Default rejects whiteouts; the explicit flag enables the existing restricted policy. Counts describe objects present or absent when individually checked; concurrent writers can make this distinction advisory. Only verified opaque CAS objects may remain after an error; there is no tag publication, external request, execution or provenance claim. Source paths must be absolute and non-symlink; changing directories or source blob identity is rejected, not an immutable filesystem snapshot. No nested indexes or foreign layers.

Tests verify raw/gzip layers, exact source index/manifest pins, whiteout replacement, source corruption, unsupported duplicate/symlink descriptors, wrong DiffID, cache reuse and clean failed staging. Actual offline CLI fixture passed with an unrelated seed blob and four selected descriptors.

```sh
bash scripts/layout-import-smoke.sh
```

## Export a pinned manifest to a portable OCI layout

```sh
gcontainers image export-layout EXISTING_STORE MANIFEST_SHA SIZE PRIVATE_PARENT [--whiteouts] --experimental
```

Select one exact manifest, copy only the verified manifest/config/layer graph into a new random owner-private layout under an existing separate private parent. No source store mutation or tags, no network and no execution. Each copied blob is size/hash checked, each decoded layer DiffID and restricted archive framing is checked, and the complete layout is independently verified. The optional whiteout policy validates removal-marker syntax but does not apply overlay semantics at export. The index is generated deterministically for the selected manifest. Retain the returned index pin independently. Partial private output on an error is preserved for review; it is not a successful export. A failure before index publication leaves no index. A later verifier failure may leave a complete-looking but unaccepted directory. Never treat directory existence alone as success.

Actual CLI test: import pinned raw/gzip whiteout layout, export the four selected blobs without the unrelated store seed, verify the portable layout, reimport into a distinct store and compare the final inert tree. Default whiteout rejection and wrong size tests passed. No source provenance, signature, general OCI compatibility or execution authorization.
