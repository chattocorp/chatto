<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MarkdownHtml from './MarkdownHtml.svelte';

  const { Story } = defineMeta({
    title: 'UI/MarkdownHtml',
    component: MarkdownHtml,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import { renderMarkdown } from '$lib/markdown';

  const emojiHtml = renderMarkdown(
    'Hello :wave: **Great work :+1:** :heart:\n\n' +
      '> A quoted :rocket: and an unknown :custom-party:.\n\n' +
      'Literal code: `:wave:`. Unicode still works: 👋.\n\n' +
      '[:heart: Documentation](https://example.com/:wave:)'
  );

  const tableHtml = renderMarkdown(`| Release | Status | Owner | Compatibility | Notes |
| :--- | :---: | ---: | :--- | :--- |
| 0.4.8 | **Stable** | Core team | Server and bundled client | Current production release |
| 0.5.0-alpha.1 | Testing | Contributors | Mixed-version clients | Previewing the next release |`);

  const shortOrderedList = '1. Short item 1\n2. Short item 2';
  const longOrderedList = Array.from(
    { length: 12 },
    (_, index) => `${index + 1}. Long item ${index + 1}`
  ).join('\n');
  const orderedListHtml = renderMarkdown(
    `Short list\n\n${shortOrderedList}\n\nLong list\n\n${longOrderedList}`
  );
  const codeHtml = renderMarkdown(
    '```js\nconst answer = 42;\n\nconsole.log("A long line of code stays inside its block and scrolls horizontally", answer);\n```\n\n    Indented code'
  );
  const ansiHtml = renderMarkdown(
    '```ansi\n' +
      '\x1b[1;36m╔══════════════════════════════════════╗\n' +
      '║            C H A T T O                ║\n' +
      '╚══════════════════════════════════════╝\n\x1b[0m' +
      '\x1b[31m████\x1b[33m████\x1b[32m████\x1b[36m████\x1b[34m████\x1b[35m████\x1b[0m\n' +
      '\x1b[38;2;255;150;90m▀▄▀▄▀▄  Unicode + true color  ▄▀▄▀▄▀\x1b[0m\n' +
      'Drawing...\x1b[6;1H\x1b[2K\x1b[92mCursor movement works.\x1b[0m\n' +
      '```'
  );
  const quoteHtml = renderMarkdown(`Text *marked with stars* and _marked with underscores_.

> Quoted text *marked with stars* and _marked with underscores_.
>
> Second paragraph with **strong**, __also strong__, \`inline code\`, and a [link](https://example.com).
>
> > Nested quote with *emphasis* and **strong** text.
>
> - Bullet with _emphasis_
> - Bullet with **strong** text
>
> 1. Ordered item with *emphasis*
> 2. Ordered item with __strong__ text`);
  const footnoteHtml =
    renderMarkdown(`The release is ready[^checks]. Read the deployment note[^deploy]. The checks are complete[^checks].

[^checks]: **All checks passed**, including the browser tests.
[^deploy]: Deploy after the backup completes.

    Keep the previous backup until the final check is complete.`);
</script>

<Story name="Emoji shortcodes" asChild>
  <div class="w-xl max-w-full rounded-md border border-border bg-surface p-3">
    <div class="prose max-w-none min-w-0">
      {#await emojiHtml then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>

<Story name="Blockquote emphasis" asChild>
  <div class="w-xl max-w-full rounded-md border border-border bg-surface p-3">
    <div class="prose max-w-none min-w-0">
      {#await quoteHtml then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>

<Story name="GFM table" asChild>
  <div class="w-80 rounded-md border border-border bg-surface p-3">
    <div class="prose max-w-none min-w-0">
      {#await tableHtml}
        <span class="text-muted">Rendering table…</span>
      {:then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>

<Story name="Footnotes" asChild>
  <div class="flex max-w-3xl flex-wrap gap-4">
    {#await footnoteHtml}
      <span class="text-muted">Rendering footnotes…</span>
    {:then html}
      {#each ['Message', 'Markdown attachment'] as label (label)}
        <div class="w-80 rounded-md border border-border bg-surface p-3">
          <p class="mb-3 font-semibold">{label}</p>
          <div class="prose max-w-none min-w-0">
            <MarkdownHtml {html} />
          </div>
        </div>
      {/each}
    {/await}
  </div>
</Story>

<Story name="Copy code blocks" asChild>
  <div class="w-96 rounded-md border border-border bg-surface p-3">
    <div class="prose max-w-none min-w-0">
      {#await codeHtml}
        <span class="text-muted">Rendering code…</span>
      {:then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>

<Story name="ANSI art" asChild>
  <div class="w-xl max-w-full rounded-md border border-border bg-surface p-3">
    <div class="prose max-w-none min-w-0">
      {#await ansiHtml then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>

<Story name="Adaptive ordered lists" asChild>
  <div class="w-80 rounded-md border border-border bg-surface p-3">
    <div class="prose max-w-none min-w-0">
      {#await orderedListHtml}
        <span class="text-muted">Rendering list…</span>
      {:then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>
