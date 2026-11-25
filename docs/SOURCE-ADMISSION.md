# Protected source admission candidate

**Draft — not an approved merge, release, or deployment.**

The candidate joins the previously separate documentation-only main and integrated development histories without discarding either branch. Both existing MIT copyright notices are retained. This reconciliation is not a determination of rights or an expansion of an existing grant.

## Evidence and outstanding gates

- The preview module currently lists only its own Go module; no third-party runtime source is vendored in the inspected tree. Go and external OCI runtimes have independent licensing and provenance.
- Historical Git author metadata is not conclusive evidence of authorship, ownership or contributor consent. Review any copied/generated/specification-derived material and record actual applicable rights before marking issue #13 accepted.
- Keep the existing required `source-admission` workflow and preview test workflow intact. Read both checks at the exact candidate commit. Main remains protected; no direct push or bypass is authorized.
- Source admission is distinct from Stable acceptance. All runtime confinement, immutable bundle, registry, rollback, MIPS, and isolated VPS Mesh gates remain open.
- The draft is intended to resolve the technical README/LICENSE conflicts for review while preserving original Git ancestry and notices. Do not merge while source rights remain unverified.

References: [foundation #1](https://github.com/GoreeCloud/containers/pull/1), [integrated preview #10](https://github.com/GoreeCloud/containers/pull/10), [branch protection #11](https://github.com/GoreeCloud/containers/issues/11), [license and provenance #13](https://github.com/GoreeCloud/containers/issues/13).
