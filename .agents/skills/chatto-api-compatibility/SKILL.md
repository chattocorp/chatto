---
name: chatto-api-compatibility
description: Review Chatto public API and protocol changes for compatibility, migration needs, and generated output.
---

# Chatto API Compatibility

Read root `AGENTS.md` for the current release policy. Follow
[proto/AGENTS.md](../../../../proto/AGENTS.md) for shared API rules and the
affected package's instructions for local requirements. Read ADR-045 for the
decision history. Do not repeat these policies in the review.

1. Compare affected source with the target branch. For release work, also
   compare with the relevant released version.
2. Classify public changes as additive, behavioral, deprecated, or breaking.
3. State the effect on existing clients and stored data. Apply the current
   release policy; do not add compatibility work that it does not require.
4. Check behavior that a schema comparison cannot verify: authorization,
   errors, absence, validation, pagination, cursors, ordering, retries, and
   realtime negotiation or recovery.
5. Check generated clients, public documentation, and relevant tests. Run Buf
   breaking checks and tests for changed behavior. A passing schema check does
   not prove behavioral compatibility.

Keep public API changes separate from persisted `chatto.core` contracts.
Review the local Operator API separately because it grants root-level access.

Report the classification, affected callers, required migration actions,
checks run, and unresolved risks. For a breaking public change, check the
approval, PR label, and documentation requirements in `proto/AGENTS.md`.
