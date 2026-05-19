<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- generic component with dynamic routes */
  import { page } from '$app/state';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

  type NavItem = { href: string; label: string; icon: string };

  let {
    title,
    subtitle,
    items,
    backHref,
    backLabel = 'Back to Chat',
    isActive = defaultIsActive,
    showMobileNav = false
  }: {
    title: string;
    subtitle?: string;
    items: NavItem[];
    backHref: string;
    backLabel?: string;
    isActive?: (href: string, items: NavItem[]) => boolean;
    showMobileNav?: boolean;
  } = $props();

  function defaultIsActive(href: string, items: NavItem[]): boolean {
    // First item gets exact match, others get prefix match
    const isFirstItem = items[0]?.href === href;
    if (isFirstItem) {
      return page.url.pathname === href;
    }
    return page.url.pathname.startsWith(href);
  }
</script>

<PaneHeader {title} {subtitle} {backHref} {backLabel} {showMobileNav} />

<nav class="sidebar-nav flex-1 p-2">
  {#each items as item (item.href)}
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- generic component with dynamic routes -->
    <a
      href={item.href}
      class={['sidebar-item', isActive(item.href, items) ? 'bg-surface-100' : '']}
    >
      <span class="sidebar-icon {item.icon}"></span>
      {item.label}
    </a>
  {/each}
</nav>
