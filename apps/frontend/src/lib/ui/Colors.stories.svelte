<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Foundations/Colors',
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  type Swatch = { name: string; cssVar: string; bgClass: string };

  const surface: Swatch[] = [
    { name: 'background', cssVar: '--color-background', bgClass: 'bg-background' },
    { name: 'surface', cssVar: '--color-surface', bgClass: 'bg-surface' },
    {
      name: 'surface-emphasized',
      cssVar: '--color-surface-emphasized',
      bgClass: 'bg-surface-emphasized'
    },
    { name: 'surface-strong', cssVar: '--color-surface-strong', bgClass: 'bg-surface-strong' },
    {
      name: 'surface-selected',
      cssVar: '--color-surface-selected',
      bgClass: 'bg-surface-selected'
    },
    { name: 'border', cssVar: '--color-border', bgClass: 'bg-border' },
    { name: 'input', cssVar: '--color-input', bgClass: 'bg-input' },
    { name: 'input-border', cssVar: '--color-input-border', bgClass: 'bg-input-border' }
  ];

  const semantic: Swatch[] = [
    { name: 'action', cssVar: '--color-action', bgClass: 'bg-action' },
    { name: 'action-hover', cssVar: '--color-action-hover', bgClass: 'bg-action-hover' },
    {
      name: 'neutral-action',
      cssVar: '--color-neutral-action',
      bgClass: 'bg-neutral-action'
    },
    { name: 'link', cssVar: '--color-link', bgClass: 'bg-link' },
    { name: 'success', cssVar: '--color-success', bgClass: 'bg-success' },
    { name: 'warning', cssVar: '--color-warning', bgClass: 'bg-warning' },
    { name: 'danger', cssVar: '--color-danger', bgClass: 'bg-danger' },
    { name: 'server', cssVar: '--color-server', bgClass: 'bg-server' }
  ];

  const presence: Swatch[] = [
    {
      name: 'presence-online',
      cssVar: '--color-presence-online',
      bgClass: 'bg-presence-online'
    },
    {
      name: 'presence-away',
      cssVar: '--color-presence-away',
      bgClass: 'bg-presence-away'
    },
    {
      name: 'presence-do-not-disturb',
      cssVar: '--color-presence-do-not-disturb',
      bgClass: 'bg-presence-do-not-disturb'
    },
    {
      name: 'presence-offline',
      cssVar: '--color-presence-offline',
      bgClass: 'bg-presence-offline'
    },
    {
      name: 'presence-invisible',
      cssVar: '--color-presence-invisible',
      bgClass: 'bg-presence-invisible'
    }
  ];

  const text: Swatch[] = [
    { name: 'text', cssVar: '--color-text', bgClass: 'bg-text' },
    { name: 'text-top', cssVar: '--color-text-top', bgClass: 'bg-text-top' },
    { name: 'muted', cssVar: '--color-muted', bgClass: 'bg-muted' }
  ];
</script>

<Story name="All tokens" asChild>
  <p class="mb-6 max-w-prose text-sm text-muted">
    Semantic color tokens defined in <code class="font-mono">src/app.css</code>. Switch the Theme
    toolbar above to compare light vs dark.
  </p>

  <section class="mb-8">
    <h2 class="mb-3 text-sm font-semibold text-muted uppercase">Surfaces</h2>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
      {#each surface as s (s.name)}
        <div class="overflow-hidden rounded-lg border border-border">
          <div class="h-16 {s.bgClass}"></div>
          <div class="p-3">
            <div class="font-mono text-sm">{s.name}</div>
            <div class="font-mono text-xs text-muted">{s.cssVar}</div>
          </div>
        </div>
      {/each}
    </div>
  </section>

  <section class="mb-8">
    <h2 class="mb-3 text-sm font-semibold text-muted uppercase">Presence</h2>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-5">
      {#each presence as s (s.name)}
        <div class="overflow-hidden rounded-lg border border-border">
          <div class="h-16 {s.bgClass}"></div>
          <div class="p-3">
            <div class="font-mono text-sm">{s.name}</div>
            <div class="font-mono text-xs text-muted">{s.cssVar}</div>
          </div>
        </div>
      {/each}
    </div>
  </section>

  <section class="mb-8">
    <h2 class="mb-3 text-sm font-semibold text-muted uppercase">Semantic</h2>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
      {#each semantic as s (s.name)}
        <div class="overflow-hidden rounded-lg border border-border">
          <div class="h-16 {s.bgClass}"></div>
          <div class="p-3">
            <div class="font-mono text-sm">{s.name}</div>
            <div class="font-mono text-xs text-muted">{s.cssVar}</div>
          </div>
        </div>
      {/each}
    </div>
  </section>

  <section class="mb-8">
    <h2 class="mb-3 text-sm font-semibold text-muted uppercase">Text</h2>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
      {#each text as s (s.name)}
        <div class="overflow-hidden rounded-lg border border-border">
          <div class="h-16 {s.bgClass}"></div>
          <div class="p-3">
            <div class="font-mono text-sm">{s.name}</div>
            <div class="font-mono text-xs text-muted">{s.cssVar}</div>
          </div>
        </div>
      {/each}
    </div>
  </section>
</Story>

<Story name="Text on surfaces" asChild>
  <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
    <div class="rounded-lg border border-border bg-background p-4">
      <div class="font-semibold">bg-background</div>
      <div class="text-text">text-text</div>
      <div class="text-muted">text-muted</div>
      <span class="text-link">text-link</span>
    </div>
    <div class="rounded-lg border border-border bg-surface p-4">
      <div class="font-semibold">bg-surface</div>
      <div class="text-text">text-text</div>
      <div class="text-muted">text-muted</div>
      <span class="text-link">text-link</span>
    </div>
    <div class="rounded-lg border border-border bg-surface-emphasized p-4">
      <div class="font-semibold">bg-surface-emphasized</div>
      <div class="text-text">text-text</div>
      <div class="text-muted">text-muted</div>
    </div>
    <div class="rounded-lg border border-border bg-surface-selected p-4">
      <div class="font-semibold">bg-surface-selected</div>
      <div class="text-text">text-text</div>
      <div class="text-muted">text-muted</div>
    </div>
  </div>
</Story>

<Story name="Semantic + opacity tints" asChild>
  <p class="mb-3 max-w-prose text-sm text-muted">
    Semantic colors are commonly used at low opacity for subtle background tints (e.g. <code
      >bg-warning/10</code
    >
    for the warning badge in <code>StatCard</code>).
  </p>
  <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
    <div class="rounded-lg bg-success/10 p-4 text-success">
      <div class="font-semibold">success</div>
      <div class="text-sm">/10 tint + text-success</div>
    </div>
    <div class="rounded-lg bg-warning/10 p-4 text-warning">
      <div class="font-semibold">warning</div>
      <div class="text-sm">/10 tint + text-warning</div>
    </div>
    <div class="rounded-lg bg-danger/10 p-4 text-danger">
      <div class="font-semibold">danger</div>
      <div class="text-sm">/10 tint + text-danger</div>
    </div>
    <div class="rounded-lg bg-neutral-action/10 p-4 text-neutral-action">
      <div class="font-semibold">neutral action</div>
      <div class="text-sm">/10 tint + text-neutral-action</div>
    </div>
  </div>
</Story>
