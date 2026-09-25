---
name: "glossary"
description: "Maintain a project's glossary of domain terms, usually in docs/GLOSSARY.md. Use when looking up terms, adding or updating definitions, renaming concepts, or auditing a glossary against repository docs, code, and decision records for missing, stale, duplicated, or misplaced entries."
---

# Glossary

Maintain the canonical vocabulary for the current project. Prefer the repository's existing glossary location and structure. If no location is obvious, look for `docs/GLOSSARY.md`, `GLOSSARY.md`, or similarly named files before proposing a new `docs/GLOSSARY.md`.

## What the Glossary Is

- A reference for what words mean in this project's context: domain nouns, internal jargon, product concepts, renamed terms, acronyms, and abbreviations.
- A naming surface: when a recurring concept lacks a stable name, add or clarify the glossary entry so the rest of the project can converge on it.
- A compact map to deeper documentation. Link to the architecture note, ADR, FDR, RFC, product spec, rules file, or source file that owns the full detail.

## What the Glossary Is Not

- Not a tutorial. Do not explain how a system works in detail; link to the document that does.
- Not an API reference. Avoid function signatures, transport schemas, generated types, and field-by-field protocol details.
- Not a dictionary of standard technical terms. Define common terms only when the project gives them a project-specific meaning.
- Not a changelog. When a term is renamed, rewrite the entry. Mention old names only when they still appear in stable identifiers, user-facing text, config keys, data stores, or migration-relevant history.

## Discover the Local Convention

Before editing, read the existing glossary end-to-end. Preserve its location, headings, ordering style, entry format, and link style unless the user asks for a restructuring.

If the glossary does not exist yet:

1. Inspect nearby documentation and repository conventions.
2. Propose a small initial structure that matches the project. Common sections include Product, UI, Domain, Architecture, Backend, Operations, Security, Data, Integrations, or Business.
3. Create only the sections needed for the entries being added.

## Where a Term Goes

Follow the glossary's existing sections. If the structure is unclear, choose by audience:

- User-facing words go in Product, UI, Domain, or Business sections.
- Contributor-only implementation words go in Architecture, Backend, Data, Integrations, or Operations sections.
- Security and authorization words go in Security, Authorization, or Access Control sections when present.
- A visible interface element belongs with UI vocabulary even if it maps to backend concepts.

A term should live in one primary section. Cross-reference related terms instead of duplicating entries.

## Ordering

Preserve the file's current ordering convention:

- If entries are alphabetical, insert alphabetically.
- If entries are conceptual, insert near prerequisite or neighboring concepts so the reader builds the model in a useful order.
- If ordering is mixed or unclear, prefer the existing local pattern in the target section.

Do not reorder unrelated entries unless the user asks for cleanup.

## Entry Format

Prefer the repository's existing entry format. If creating a new glossary, use:

```markdown
**Term** - One-line definition. Link to the most relevant owning document if there is one.
```

- Bold the term itself.
- For acronyms, expand on first mention: `**ADR (Architecture Decision Record)** - ...`.
- Keep entries short: one line or one short paragraph.
- Cross-link rather than re-explain: `[ADR-005](adr/ADR-005-example.md)`, `[RFC-012](rfcs/012-example.md)`, or `` `src/example.ts` ``.

## How To Run

### Mode 1: Look up a term

When invoked with one or more terms:

1. Find the glossary file.
2. Search for each term case-insensitively, including aliases if the glossary has them.
3. Print matching entries verbatim with their section headings.
4. If a term is not found, suggest the closest existing entries and say whether adding a new entry looks warranted.

### Mode 2: Add or update a term

When invoked to add, define, rename, or update a term:

1. Find and read the glossary file.
2. Confirm the term or a near-duplicate does not already exist. If it does, update the existing entry instead of adding a duplicate.
3. Research the term in repository docs, decision records, rules, source code, tests, and recent commits as needed.
4. Draft a concise definition grounded in current project usage.
5. Choose the section and insertion point using the local convention.
6. Apply the edit when the requested change is clear. If the definition, canonical name, or placement is ambiguous, propose the wording first and ask the user to choose.

### Mode 3: Audit

When invoked to audit, or when invoked without a specific term:

1. Find and read the glossary file end-to-end.
2. Check cited links and referenced files for dead links or stale claims.
3. Scan relevant docs, decision records, rules, source code, tests, and recent commits for repeated project-specific terms that are missing from the glossary. Useful signals include capitalized nouns, acronyms, hyphenated phrases, quoted strings, renamed concepts, and terms used across multiple documents.
4. Limit missing-term proposals to the strongest candidates, usually around 10.
5. Report:
   - Stale entries: definitions contradicted by current code or docs.
   - Dead links: references to renamed or removed files.
   - Duplicate or overlapping entries: multiple entries for one concept.
   - Misplaced entries: entries that conflict with the glossary's section convention.
   - Missing terms: candidates worth adding, with a section and one-line draft for each.
6. Keep audits propose-only unless the user explicitly asks you to apply fixes.

## Audit Heuristics

Strong glossary candidates include:

- Words with project-specific meaning that a general dictionary or web search would not explain.
- Words that changed meaning or were renamed recently.
- Terms used across multiple docs, decision records, issues, or source areas with assumed shared meaning.
- Acronyms and abbreviations that contributors use without expansion.
- Concepts reviewers or teammates have asked about.
- Recurring noun phrases that suggest the project lacks a canonical name.

Avoid adding:

- Standard technical terms with no project-specific meaning.
- Type names, function names, file paths, and generated identifiers unless they are also stable product or architecture concepts.
- Implementation details that are better documented in code comments, API docs, or architecture docs.

## Workflow Notes

- Before any edits, read the existing glossary. Glossaries are usually short enough to load entirely.
- Use `rg` to find real usage before defining or renaming a term.
- Prefer canonical names. Mention aliases only when they are still useful for search, migration, compatibility, or user understanding.
- When glossary naming conflicts with code or docs, do not rename unrelated files during a glossary task. Note the conflict or pending rename unless the user asked for the broader rename.
- Keep edits narrow and avoid using glossary maintenance as a reason to refactor unrelated documentation.
