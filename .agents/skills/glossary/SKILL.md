---
name: glossary
description: Look up, add, or audit Chatto terms in docs/GLOSSARY.md.
---

# Chatto Glossary

Read [the glossary](../../../docs/GLOSSARY.md) before editing. It defines the
naming policy, section order, and entry style. Use one entry per concept.
Define Chatto-specific meanings; omit standard technical definitions, code
signatures, and tutorials.

## Tasks

- **Lookup:** Find the requested terms without case sensitivity. Return their
  entries and sections. If a term is absent, suggest the closest entries.
- **Add:** Check for duplicates. Check the meaning against code, FDRs, ADRs,
  and applicable `AGENTS.md` files. Propose a definition and section for approval
  before writing, unless the user already approved them.
- **Audit (default):** Check definitions and links against their sources.
  Report stale, missing, duplicate, or misplaced entries. Do not apply changes
  without a request to do so.

For an audit, also check decision indexes and recent changes for missing terms.
Prioritize terms used in several records or terms whose meaning changed.
Limit missing-term proposals to about ten useful candidates.

## Entries

Use a bold term followed by a short definition. Expand abbreviations on first
use. Put each term in one section: UI, Product, Authorization, or Backend.
Place foundational terms before terms that depend on them, not alphabetically.

Link to the record that explains the concept. Replace stale definitions in
place. Keep old names only when readers still need them for existing identifiers.
If code uses a different name from the canonical term, report the required
rename; a glossary task does not authorize unrelated code changes.
