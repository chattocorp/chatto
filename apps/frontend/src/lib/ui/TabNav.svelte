<!--
@component

Link navigation between the sections of one resource page, for example the
Overview, Integrations, and Permissions pages of a bot. Render it in the `tabs`
snippet of `PaneHeader`. Each tab is a real link to its own route, and the
current tab has `aria-current="page"`.

The tabs use the hit area and the active fill of the room sidebar toggles in
the pane header. When the pane header is narrower than 48 rem, the tabs show
only their icons; the labels stay available as accessible names and tooltips.

Use `SegmentedControl` instead for a view mode or filter that does not change
the route. The navigation renders nothing when fewer than two tabs are
available.
-->
<script module lang="ts">
  /** One section link in a `TabNav`. */
  export interface TabNavItem {
    /** Resolved app path of the section. */
    href: string;
    label: string;
    /** Iconify utility class. The icon represents the tab in narrow headers. */
    icon: string;
    /** True when the section is the current page. */
    current: boolean;
  }
</script>

<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- item hrefs are props; callers pass resolved app paths */
  let {
    label,
    items
  }: {
    /** Accessible name for the navigation landmark. */
    label: string;
    items: ReadonlyArray<TabNavItem>;
  } = $props();
</script>

{#if items.length > 1}
  <nav aria-label={label} class="flex shrink-0 items-center gap-1">
    {#each items as item (item.href)}
      <a
        href={item.href}
        title={item.label}
        aria-current={item.current ? 'page' : undefined}
        class={[
          'inline-flex h-10 min-w-10 shrink-0 cursor-pointer items-center justify-center gap-2 icon-action-feedback px-3 whitespace-nowrap',
          item.current && 'pane-header-icon-button-active'
        ]}
      >
        <span class={['pane-header-icon-glyph', item.icon]} aria-hidden="true"></span>
        <span class="sr-only @min-[48rem]/pane-header:not-sr-only">{item.label}</span>
      </a>
    {/each}
  </nav>
{/if}
