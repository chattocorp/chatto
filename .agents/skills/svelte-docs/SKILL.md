---
name: svelte-docs
description: Look up official Svelte, SvelteKit, and Svelte CLI documentation.
---

# Svelte Documentation

Use the documentation CLI to find and read relevant sections:

```sh
mise x -- npx @sveltejs/mcp list-sections
mise x -- npx @sveltejs/mcp get-documentation '$state,$derived,$effect'
```

Use single quotes for section names that contain `$`.

For web lookup, start with the relevant official index:

- Svelte: `https://svelte.dev/docs/svelte/llms-small.txt`
- SvelteKit: `https://svelte.dev/docs/kit/llms-small.txt`
- CLI: `https://svelte.dev/docs/cli/llms.txt`

Use an available web tool. Read full documentation only if the short version
cannot answer the question. Use `https://svelte.dev/llms-small.txt` when the
package is unknown; `llms-medium.txt` and `llms-full.txt` provide more detail.

Treat supplied text as the search topic. Honor explicit `svelte`, `kit`, or
`cli` scope and `medium` or `full` detail requests.
