---
name: "svelte-docs"
description: "Use this if you want to look up Svelte, SvelteKit, and Svelte CLI documentation."
---

# Svelte Documentation Lookup

Look up Svelte, SvelteKit, and Svelte CLI documentation from the official LLM-optimized docs at svelte.dev.

## Instructions

1. **Start with compressed documentation** to minimize context usage:

   Fetch `https://svelte.dev/llms-small.txt` using the WebFetch tool.

2. **If the answer is unclear or more detail is needed**, escalate to medium docs:

   Fetch `https://svelte.dev/llms-medium.txt`

3. **For comprehensive reference** (only when necessary):

   Fetch `https://svelte.dev/llms-full.txt`

## Package-Specific Documentation

When you know the question is about a specific package, use targeted docs:

- **Svelte only**: `https://svelte.dev/docs/svelte/llms-small.txt` or `https://svelte.dev/docs/svelte/llms.txt`
- **SvelteKit only**: `https://svelte.dev/docs/kit/llms-small.txt` or `https://svelte.dev/docs/kit/llms.txt`
- **CLI only**: `https://svelte.dev/docs/cli/llms.txt`

## Using WebFetch

When fetching documentation, use a prompt that extracts the relevant information:

```
WebFetch url: https://svelte.dev/llms-small.txt
prompt: "Find information about [topic]. Extract relevant code examples and explanations."
```

## Arguments: $ARGUMENTS

If arguments are provided, interpret them as:

- `svelte` - Fetch only Svelte docs (not SvelteKit)
- `kit` or `sveltekit` - Fetch only SvelteKit docs
- `cli` - Fetch only CLI docs
- `full` - Skip to full documentation immediately
- `medium` - Start with medium documentation
- Any other text - Use as the search topic with the default progressive strategy
