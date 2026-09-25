---
name: technical-writing
description: Invoke this skill whenever you need to create, update, review, restructure, or audit technical documentation such as README files, user manuals, API documentation, architecture notes, setup guides, troubleshooting guides, or developer docs. Use it to align docs with source code, existing project conventions, and the intended audience.
---

## Source First

- Read relevant project instructions and existing documentation before writing. Check for files such as `AGENTS.md`, `README.md`, `docs/`, `.agents/`, `.codex/`, or repository-specific documentation guidance.
- Verify technical claims against source code, schemas, configuration, command output, APIs, or authoritative references. Do not rely on plausible implementation guesses when the repository can answer the question.
- Preserve existing documentation conventions for headings, tone, formatting, examples, terminology, and file placement unless the user asks for a broader rewrite.
- When updating existing docs, rewrite stale content in place. Avoid appending correction notes unless the document intentionally keeps historical context.
- Keep the documentation scoped to what the user asked for. Link to adjacent topics instead of explaining unrelated systems in detail.

## Target Audience

- Identify the likely audience from the document type, repository context, and user request. State or apply that assumption when it affects depth, examples, or terminology.
- Ask for clarification only when the audience ambiguity would materially change the structure or technical level of the documentation.

## Language and Structure

- Use clear, direct language. Avoid jargon, complex sentences, and unnecessary technical terms.
- Use the repository's established terminology. Do not introduce synonyms for existing concepts unless you are intentionally standardizing inconsistent language.
- Organize content with headings, bullets, numbered steps, tables, or examples when they make the material easier to scan.
- Include code snippets, commands, API examples, diagrams, or screenshots only when they help the reader complete the task or understand the system.
- Avoid duplicate content. If existing documentation already covers a topic, link to it or update the source document instead of copying it into a new place.

## Review and Edit

- Proofread for grammar, spelling, formatting, and consistency.
- Verify that links, references, commands, examples, and file paths are correct and functional when practical.
- Check that the final document answers the user's request, matches the intended audience, and does not over-document auxiliary topics.
