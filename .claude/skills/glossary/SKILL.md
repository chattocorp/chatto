---
name: glossary
description: Maintain Chatto's glossary of terms in docs/GLOSSARY.md. Look up a term, add a new one, or audit the glossary against the codebase, FDRs, and ADRs for missing or stale entries.
---

# Glossary

The canonical vocabulary for Chatto. Lives at `docs/GLOSSARY.md`, organised into four sections — UI, Product, Authorization, Backend — siblings of [FDRs](../../../docs/fdr/INDEX.md) and [ADRs](../../../docs/adr/INDEX.md).

## What the Glossary Is

- A **reference** for what a word means in Chatto's context — internal jargon, product nouns, renamed concepts, acronyms.
- A **naming surface**: when there's a thing-without-a-name, we add it here first. The glossary leads, the code follows. If a file or component disagrees with the glossary, the file is what should rename.
- One line per entry, occasionally one short paragraph. Longer concepts link to the FDR/ADR/rules file that owns them.

## What the Glossary Is NOT

- **NOT** a tutorial. Don't explain *how* something works — FDRs, ADRs, and `docs/ARCHITECTURE.md` are for that.
- **NOT** an API reference. No function signatures, no GraphQL types, no proto fields.
- **NOT** a dictionary of standard tech terms. Define "Republish" (a Chatto-specific use of a JetStream feature). Don't define "PostgreSQL" or "WebSocket".
- **NOT** a changelog. When a term is renamed, rewrite the entry. Don't append "previously known as…" unless old names still appear in stable identifiers (stream names, KV keys, config keys).

## Sections

1. **UI** — visible surfaces and component groupings (Server Gutter, Server Sidebar, Room View, Room Sidebar, Composer, Pane Header, etc.).
2. **Product** — user-facing concepts. If a user might say the word, it goes here (Server, Space, Room, DM, Thread, Mention, Reaction, …).
3. **Authorization** — RBAC vocabulary. Roles, permissions, ranks, scopes (Role, Permission, Position, Rank, Outranking, Owner/Admin/Moderator/Everyone, Scope, DM Privacy Boundary).
4. **Backend** — infrastructure jargon. If only contributors say the word, it goes here (ChattoCore, NATS, JetStream, Stream, KV, Subject, Live Event, OCC, Nanoid, Crypto-shredding).

### Where does a term go?

Rule of thumb: **section by who-uses-the-word.**

- Users say it → Product or UI.
- Only contributors say it → Backend or Authorization.
- It names a visible thing → UI.

Apply the rule once and don't re-litigate. A term lives in one section. Cross-reference within an entry's body if useful (e.g. *Echo* in Product references the `message.echo` permission, which lives in Authorization implicitly).

## Within a section: conceptual order, not alphabetical

Order foundational terms first, derivatives after. Reader builds up the model in the order it makes sense.

- **Authorization** opens with RBAC → Role → Permission → Position → Rank → Outranking. Then the named roles (Owner, Admin, Moderator, Everyone). Then edge concepts (Scope, User-level override, DM Privacy Boundary).
- **Product** opens with the structural nouns (Server → Space → Room → Room Group → DM), then content (Message, Thread, Echo, Reaction, Mention, Attachment, …), then ambient features (Presence, Typing Indicator, …).
- **UI** opens with the named sidebars in left-to-right reading order (Server Gutter → Server Sidebar → Room View → Room Sidebar), then in-pane elements (Composer, Pane Header), then secondary surfaces (Quick Switcher, Slideover, Hint, Panel).
- **Backend** opens with ChattoCore (the API/core boundary), then NATS/JetStream/Stream/KV/Subject (the storage substrate), then derived concepts (Event, Live Event, Republish, OCC, Nanoid, Crypto-shredding).

When adding a new entry: insert at the position its dependencies suggest, not alphabetically.

## Entry Format

```markdown
**Term** — One-line definition. Link to the relevant FDR/ADR/rules file if there is one.
```

- Bold the term itself.
- For acronyms, expand on first mention: `**FDR (Feature Decision Record)** — …`.
- Cross-link rather than re-explain: `[FDR-007](fdr/FDR-007-direct-messages.md)`, `[ADR-005](adr/ADR-005-hierarchy-wins-rbac.md)`, `` `.claude/rules/authorization.md` ``.

## How To Run

### Mode 1: Look up a term

When invoked with one or more terms (e.g. `/glossary echo` or `/glossary echo, primary space`):

1. Search `docs/GLOSSARY.md` for each term (case-insensitive).
2. Print the matching entries verbatim, along with the section they belong to.
3. If a term isn't found, suggest the closest existing entries and offer to add one.

### Mode 2: Add a term

When invoked with `add <term>` (e.g. `/glossary add room sharding`):

1. Confirm the term doesn't already exist. A near-duplicate is worth flagging — propose updating the existing entry instead.
2. Research the term in the codebase, FDRs, ADRs, and `.claude/rules/` so the definition is grounded.
3. Draft a one-line definition. Decide which section it belongs in using the rule-of-thumb above. Offer both to the user for approval before writing.
4. Insert in conceptual order within the chosen section. Cross-link any related FDR/ADR/rules file.

### Mode 3: Audit (default, no args)

When invoked without arguments:

1. Read `docs/GLOSSARY.md` end-to-end.
2. Cross-reference each entry against its cited FDR/ADR/rules file — flag dead links or claims that no longer hold.
3. Scan `docs/fdr/INDEX.md`, `docs/adr/INDEX.md`, `.claude/rules/`, and recent commits for terms used as jargon but not in the glossary (heuristic: capitalised nouns, hyphenated phrases, repeated quoted strings). Limit the proposal list to ~10 strongest candidates.
4. Report:
   - **Stale entries** — definitions contradicted by current code/docs.
   - **Dead links** — references to renamed or removed files.
   - **Misplaced entries** — terms that belong in a different section per the rule-of-thumb.
   - **Missing terms** — top candidates worth adding, with a section + one-line draft for each.
5. **Propose-only**: don't apply edits without user approval.

## Audit Heuristics

When scouting for missing terms:

- Words that **changed meaning** recently (e.g. *Server* used to be *Instance*) almost always deserve an entry.
- Words that appear across **multiple FDRs/ADRs** with assumed shared meaning are strong candidates.
- Acronyms and abbreviations (RBAC, OCC, DM, KV, FDR/ADR, …).
- Words a reviewer asked about in PR review — if one human had to ask, others will.
- Things-without-names: a recurring noun phrase ("the right pane", "the small column on the left") signals a name is overdue. Naming such things is one of the glossary's jobs.

When *not* to add:

- Standard tech terms with no Chatto-specific meaning ("PostgreSQL", "WebSocket").
- Type names, function names, file paths — those belong in code, not a glossary.
- Anything one Google search would answer.

## Workflow Notes

- **Before any edits**: read the existing `docs/GLOSSARY.md`. The file is short enough to load entirely.
- **Cross-link aggressively**: a glossary entry is most useful as a jumping-off point. If there's an FDR or ADR for the term, link it.
- **One entry per concept**, even with multiple names. Pick the canonical name as the heading; mention aliases in the body if useful (e.g. `**Server** — Top-level Chatto deployment. Formerly called *Instance* in the codebase.`).
- **Glossary > code on naming**: when an entry contradicts a component or file name in the codebase, note "(pending rename)" in the entry. The follow-up is to rename the code, not the entry.
