<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- admin hrefs are resolved before they are passed in */
  import type { AdminNavItem } from './adminNav';

  let { currentPath, expandedByDefault, items }: {
    currentPath: string;
    expandedByDefault: boolean;
    items: AdminNavItem[];
  } = $props();

  let expansionOverride = $state<boolean | null>(null);
  const expanded = $derived(expansionOverride ?? expandedByDefault);

  function isActive(href: string): boolean {
    return currentPath.startsWith(href);
  }
</script>

{#if items.length > 0}
  <button
    type="button"
    class="sidebar-item text-left"
    aria-expanded={expanded}
    aria-controls="server-admin-sidebar-links"
    onclick={() => {
      expansionOverride = !expanded;
    }}
  >
    <span class="sidebar-icon iconify uil--setting"></span>
    <span class="min-w-0 flex-1 truncate">Administration</span>
    <span
      class={[
        'iconify uil--angle-right-b shrink-0 text-base text-muted transition-transform',
        expanded ? 'rotate-90' : ''
      ]}
    ></span>
  </button>
  {#if expanded}
    <div id="server-admin-sidebar-links" class="sidebar-nav">
      {#each items as item (item.href)}
        <a
          href={item.href}
          class={['sidebar-item pl-8', isActive(item.href) ? 'bg-surface-100' : '']}
        >
          <span class="sidebar-icon {item.icon}"></span>
          {item.label}
        </a>
      {/each}
    </div>
  {/if}
{/if}
