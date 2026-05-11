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
    { name: 'surface-100', cssVar: '--color-surface-100', bgClass: 'bg-surface-100' },
    { name: 'surface-200', cssVar: '--color-surface-200', bgClass: 'bg-surface-200' },
    { name: 'surface-300', cssVar: '--color-surface-300', bgClass: 'bg-surface-300' },
    {
      name: 'surface-highlighted',
      cssVar: '--color-surface-highlighted',
      bgClass: 'bg-surface-highlighted'
    },
    { name: 'border', cssVar: '--color-border', bgClass: 'bg-border' },
    { name: 'input', cssVar: '--color-input', bgClass: 'bg-input' },
    { name: 'input-border', cssVar: '--color-input-border', bgClass: 'bg-input-border' }
  ];

  const semantic: Swatch[] = [
    { name: 'primary', cssVar: '--color-primary', bgClass: 'bg-primary' },
    { name: 'primary-hover', cssVar: '--color-primary-hover', bgClass: 'bg-primary-hover' },
    { name: 'accent', cssVar: '--color-accent', bgClass: 'bg-accent' },
    { name: 'link', cssVar: '--color-link', bgClass: 'bg-link' },
    { name: 'success', cssVar: '--color-success', bgClass: 'bg-success' },
    { name: 'warning', cssVar: '--color-warning', bgClass: 'bg-warning' },
    { name: 'danger', cssVar: '--color-danger', bgClass: 'bg-danger' },
    { name: 'server', cssVar: '--color-server', bgClass: 'bg-server' }
  ];

  const text: Swatch[] = [
    { name: 'text', cssVar: '--color-text', bgClass: 'bg-text' },
    { name: 'muted', cssVar: '--color-muted', bgClass: 'bg-muted' }
  ];
</script>

<Story name="All tokens" asChild>
  <p class="mb-6 max-w-prose text-sm text-muted">
    Semantic color tokens defined in <code class="font-mono">src/app.css</code>.
    Switch the Theme toolbar above to compare light vs dark.
  </p>

  <section class="mb-8">
    <h2 class="mb-3 text-sm font-semibold uppercase text-muted">Surfaces</h2>
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
    <h2 class="mb-3 text-sm font-semibold uppercase text-muted">Semantic</h2>
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
    <h2 class="mb-3 text-sm font-semibold uppercase text-muted">Text</h2>
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
    <div class="rounded-lg border border-border bg-surface-200 p-4">
      <div class="font-semibold">bg-surface-200</div>
      <div class="text-text">text-text</div>
      <div class="text-muted">text-muted</div>
    </div>
    <div class="rounded-lg border border-border bg-surface-highlighted p-4">
      <div class="font-semibold">bg-surface-highlighted</div>
      <div class="text-text">text-text</div>
      <div class="text-muted">text-muted</div>
    </div>
  </div>
</Story>

<Story name="Semantic + opacity tints" asChild>
  <p class="mb-3 max-w-prose text-sm text-muted">
    Semantic colors are commonly used at low opacity for subtle background
    tints (e.g. <code>bg-warning/10</code> for the warning badge in <code>StatCard</code>).
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
    <div class="rounded-lg bg-primary/10 p-4 text-primary">
      <div class="font-semibold">primary</div>
      <div class="text-sm">/10 tint + text-primary</div>
    </div>
  </div>
</Story>
