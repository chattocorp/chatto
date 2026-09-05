---
name: svelte-code-writer
description: Use the Svelte CLI to check any Svelte component or module that you create, edit, or analyze.
---

# Svelte Code Checks

Follow [svelte-core-bestpractices](../svelte-core-bestpractices/SKILL.md) for
Svelte patterns. Use [svelte-docs](../svelte-docs/SKILL.md) for documentation.

Run the autofixer when reviewing or debugging Svelte code, and before returning
changes to a `.svelte`, `.svelte.ts`, or `.svelte.js` file:

```sh
mise x -- npx @sveltejs/mcp svelte-autofixer ./path/to/Component.svelte
```

Use `--svelte-version 4` only for Svelte 4 code. Use `--async` when checking
async Svelte mode. Fix the cause of reported warnings.

Prefer a file path to inline source. If inline source contains `$` runes, use
shell single quotes. Do not add backslashes to runes inside single quotes.
