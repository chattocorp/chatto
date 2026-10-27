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
    '```ruby\nIO.puts("this is a test")\n```\n\n```js\nconst answer = 42;\n\nconsole.log("A long line of code stays inside its block and scrolls horizontally", answer);\n```\n\n    Indented code'
  );
  const sourceTabsHtml = renderMarkdown(
    'README example:\n\n```mermaid\nflowchart LR\nA[Source first] --> B[Select Render]\n```'
  );
  const longSourceHtml = renderMarkdown(
    '```svg\n<svg xmlns="http://www.w3.org/2000/svg" width="600" height="600" viewBox="0 0 600 600">\n' +
      Array.from(
        { length: 64 },
        (_, i) =>
          `  <circle cx="${38 + (i % 8) * 75}" cy="${38 + Math.floor(i / 8) * 75}" r="24" fill="#55aabb" />`
      ).join('\n') +
      '\n</svg>\n```'
  );
  const ansiHtml = renderMarkdown(
    '```ansi rendered\n' +
      '\x1b[1;36m╔══════════════════════════════════════╗\n' +
      '║            C H A T T O                ║\n' +
      '╚══════════════════════════════════════╝\n\x1b[0m' +
      '\x1b[31m████\x1b[33m████\x1b[32m████\x1b[36m████\x1b[34m████\x1b[35m████\x1b[0m\n' +
      '\x1b[38;2;255;150;90m▀▄▀▄▀▄  Unicode + true color  ▄▀▄▀▄▀\x1b[0m\n' +
      'Drawing...\x1b[6;1H\x1b[2K\x1b[92mCursor movement works.\x1b[0m\n' +
      '```'
  );
  const svgHtml = renderMarkdown(
    '```svg rendered\n<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 480 180" width="480" height="180">\n' +
      '<rect width="480" height="180" rx="16" fill="#edf7f4"/>\n' +
      '<circle cx="90" cy="90" r="48" fill="#257d68"/>\n' +
      '<path d="M65 90l18 18 34-38" fill="none" stroke="white" stroke-width="8"/>\n' +
      '<text x="165" y="86" font-family="sans-serif" font-size="28" fill="#163e34">All systems ready</text>\n' +
      '<text x="165" y="118" font-family="sans-serif" font-size="18" fill="#257d68">SVG from a bot message</text>\n</svg>\n```'
  );
  const mermaidHtml = renderMarkdown(
    '```mermaid rendered\nflowchart LR\naccTitle: Orbital delivery\naccDescr: Survey the station, dock, trade cargo, then depart.\nA[Survey station] --> B{Docking clearance?}\nB -->|Granted| C[Trade cargo]\nB -->|Denied| D[Wait in orbit]\nD --> B\nC --> E[Depart]\n```'
  );
  const mermaidFallbackHtml = renderMarkdown(
    '```mermaid rendered\nThis diagram is incomplete\n```'
  );
  const mermaidTallHtml = renderMarkdown(
    '```mermaid rendered\nflowchart TB\naccTitle: Launch checklist\nA[Clear customs] --> B[Load medicine]\nB --> C[Seal cargo hold]\nC --> D[Refuel tanks]\nD --> E[Test engines]\nE --> F[Upload flight plan]\nF --> G[Request clearance]\nG --> H[Release docking clamps]\nH --> I[Depart for Haven]\n```'
  );
  const mermaidGroupsHtml = renderMarkdown(
    '```mermaid rendered\nflowchart TB\naccTitle: Flight operations\nsubgraph Trade\nA[Buy medicine] --> B[Seal manifest]\nend\nsubgraph Engineering\nC[Refuel tanks] --> D[Systems green]\nend\nB --> E{Launch clearance}\nD --> E\nE -->|Approved| F[Depart for Haven]\nE -.->|Hold| G[Wait at Kepler]\n```'
  );
  const mermaidThemeHtml = renderMarkdown(
    [
      '```mermaid rendered\nsequenceDiagram\naccTitle: Docking handshake\nparticipant S as Kestrel\nparticipant P as Kepler Port\nS->>P: Request clearance\nP-->>S: Dock 04 is ready\nS->>P: Begin approach\n```',
      '```mermaid rendered\npie title Cargo manifest\n"Medicine" : 42\n"Ore" : 28\n"Supplies" : 18\n"Fuel cells" : 12\n```',
      '```mermaid rendered\nclassDiagram\nclass Ship {\n+String callsign\n+int cargoCapacity\n+dock()\n}\nclass Port {\n+String name\n+requestClearance()\n}\nShip --> Port : visits\n```'
    ].join('\n\n')
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

<Story name="Source and Render tabs" asChild>
  <div class="prose w-xl max-w-full">
    {#await sourceTabsHtml then html}
      <MarkdownHtml {html} />
    {/await}
  </div>
</Story>

<Story name="Scrollable source" asChild>
  <div class="prose w-2xl max-w-full">
    {#await longSourceHtml then html}
      <MarkdownHtml {html} />
    {/await}
  </div>
</Story>

<Story name="Highlighted code frame" asChild>
  <div class="message-row bg-surface">
    <div class="prose w-xl max-w-full">
      {#await sourceTabsHtml then html}
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
  <div class="w-2xl max-w-full">
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

<Story name="SVG preview" asChild>
  <div class="w-xl max-w-full">
    <div class="prose max-w-none min-w-0">
      {#await svgHtml then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>

<Story name="Mermaid preview" asChild>
  <div class="prose w-xl max-w-full min-w-0">
    {#await mermaidHtml then html}<MarkdownHtml {html} />{/await}
  </div>
</Story>

<Story name="Mermaid fallback" asChild>
  <div class="prose w-80 max-w-full min-w-0">
    {#await mermaidFallbackHtml then html}<MarkdownHtml {html} />{/await}
  </div>
</Story>

<Story name="Mermaid tall diagram" asChild>
  <div class="prose w-80 max-w-full min-w-0">
    {#await mermaidTallHtml then html}
      <MarkdownHtml {html} />
    {/await}
  </div>
</Story>

<Story name="Mermaid diagram types" asChild>
  <div class="prose w-2xl max-w-full min-w-0">
    {#await mermaidThemeHtml then html}
      <MarkdownHtml {html} />
    {/await}
  </div>
</Story>

<Story name="Mermaid groups" asChild>
  <div class="prose w-2xl max-w-full min-w-0">
    {#await mermaidGroupsHtml then html}
      <MarkdownHtml {html} />
    {/await}
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
