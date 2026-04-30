---
paths: ["frontend/**"]
---

# Storybook

The frontend ships a Storybook design-system catalog under
`frontend/.storybook/` and `*.stories.svelte` files co-located next to the
components they document.

## When to add or update a story

- **New reusable component** in `src/lib/ui/`, `src/lib/ui/form/`, or
  `src/lib/components/admin/` → add a `*.stories.svelte` next to it.
- **Changed component prop API or visual variants** → update its story so
  the variants shown match the current API. The variant stories ("All
  tones", "Variants", "Sizes") are the documentation; keep them current.
- **Changed design tokens or utility classes** in `src/app.css` → reflect
  in the `Foundations/Colors` or `Foundations/Typography` stories.

## addon-svelte-csf v5 conventions

- **Always pass `asChild` on `<Story>` blocks that contain markup.** Without
  it, svelte-csf passes the body as the `children` snippet of the meta's
  `component`, which silently disappears for components that don't render
  children (e.g. `<SkeletonImg>`).

  ```svelte
  <Story name="All tones" asChild>
    <div class="flex gap-2">
      {#each tones as t (t)}<Pill tone={t}>{t}</Pill>{/each}
    </div>
  </Story>
  ```

- **Avoid the `template={fn}` + `args` pattern** for "Default" stories. It
  doesn't type-check cleanly when the component has required `children`,
  and the variant stories already document the API better than a Controls
  playground would. Prefer plain `asChild` markup.

- **Story file naming**: `Foo.stories.svelte` next to `Foo.svelte`. Title
  the meta with the section the file lives in: `'UI/Foo'`, `'Form/Foo'`,
  `'Components/Foo'`, `'Demos/...'`, `'Foundations/...'`. The story sort
  order in `frontend/.storybook/preview.ts` expects these prefixes.

## Theming

- `data-theme` on `<html>` is the single switch for app theme. Set in prod
  by the inline script in `src/app.html`, set in Storybook by the decorator
  in `.storybook/preview.ts`. The toolbar global (`Auto`/`Light`/`Dark`)
  flips it; `Shift+T` cycles.
- **Don't add `@media (prefers-color-scheme: dark)` rules** in component
  CSS. The inline script handles system preference and writes
  `data-theme`; CSS only needs `:root[data-theme='dark']` (or a bare
  `[data-theme='dark']` on a wrapper if scoping a region).
- Storybook's own UI (manager + docs chrome) is themed via Storybook's
  presets, not our tokens. We deliberately do **not** retint
  `.sbdocs-wrapper` / `.sb-argstableBlock` / etc. — only the embed canvas
  (`.sbdocs-preview`) takes our `--color-background`.
