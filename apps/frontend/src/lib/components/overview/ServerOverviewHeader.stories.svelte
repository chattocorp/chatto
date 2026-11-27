<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { ComponentProps } from 'svelte';
  import ServerOverviewHeader from './ServerOverviewHeader.svelte';

  const { Story } = defineMeta({
    title: 'Overview/ServerOverviewHeader',
    component: ServerOverviewHeader,
    parameters: { layout: 'fullscreen' }
  });

  /** A 1200×630 banner with a centred motif and detail near every edge, so crops are obvious. */
  const banner = `data:image/svg+xml,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630"><rect width="1200" height="630" fill="#c39bd3"/><rect y="0" width="1200" height="90" fill="#a678bd"/><rect y="540" width="1200" height="90" fill="#a678bd"/><rect x="380" y="80" width="440" height="470" rx="60" fill="#dce6f5"/><path d="M440 200 500 60 560 200Z M640 200 700 60 760 200Z" fill="#6b8fbd"/><circle cx="600" cy="360" r="170" fill="#fff"/><circle cx="540" cy="380" r="22" fill="#6b8fbd"/><rect x="600" y="370" width="90" height="16" rx="8" fill="#6b8fbd"/><text x="600" y="610" font-family="sans-serif" font-size="34" font-weight="700" fill="#fff" text-anchor="middle">CHATTO HQ</text></svg>'
  )}`;

  /** A banner whose left and right edges differ, to show that the backdrop follows each edge. */
  const twoToneBanner = `data:image/svg+xml,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630"><defs><linearGradient id="g"><stop offset="0" stop-color="#1f4e5f"/><stop offset="1" stop-color="#e3a857"/></linearGradient></defs><rect width="1200" height="630" fill="url(#g)"/><circle cx="600" cy="315" r="200" fill="#f6efe3"/></svg>'
  )}`;

  const logo = `data:image/svg+xml,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128"><rect width="128" height="128" rx="28" fill="#c39bd3"/><rect x="30" y="30" width="68" height="68" rx="14" fill="#dce6f5"/><circle cx="64" cy="70" r="20" fill="#fff"/></svg>'
  )}`;

  const server: ComponentProps<typeof ServerOverviewHeader> = {
    name: 'Chatto HQ',
    logoUrl: logo,
    bannerUrl: banner,
    description: 'The Chatto metacommunity. Come chat with us... about Chatto!'
  };

  const widths = [
    { label: 'Wide pane', className: 'max-w-6xl' },
    { label: 'Medium pane', className: 'max-w-2xl' },
    { label: 'Phone', className: 'max-w-sm' }
  ];
</script>

{#snippet atWidths(props: typeof server)}
  <div class="flex flex-col gap-8 p-6">
    <!--
      Each sample is a labelled section. Outside a section, each header is a
      page banner landmark, and a page must have only one.
    -->
    {#each widths as width (width.label)}
      <section class="flex flex-col gap-2" aria-label={width.label}>
        <span class="text-sm text-muted" aria-hidden="true">{width.label}</span>
        <div class={['w-full', width.className]}>
          <ServerOverviewHeader {...props} />
        </div>
      </section>
    {/each}
  </div>
{/snippet}

<Story name="Banner" asChild>
  {@render atWidths(server)}
</Story>

<Story name="Two-tone banner" asChild>
  {@render atWidths({ ...server, bannerUrl: twoToneBanner })}
</Story>

<Story name="No banner" asChild>
  {@render atWidths({ ...server, bannerUrl: null })}
</Story>

<Story name="No banner or logo" asChild>
  {@render atWidths({ ...server, bannerUrl: null, logoUrl: null, description: null })}
</Story>
