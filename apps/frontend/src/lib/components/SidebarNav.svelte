<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- generic component with dynamic routes */
  import { page } from '$app/state';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { m } from '$lib/i18n/messages';

  export type NavItem = { href: string; label: string; icon: string };
  export type NavGroup = { label: string; items: NavItem[] };

  let {
    title,
    subtitle,
    items = [],
    groups = [],
    backHref,
    backLabel = m('ui.sidebar_nav.back_to_chat'),
    isActive = defaultIsActive,
    showMobileNav = false
  }: {
    title: string;
    subtitle?: string;
    items?: NavItem[];
    groups?: NavGroup[];
    backHref: string;
    backLabel?: string;
    isActive?: (href: string, items: NavItem[]) => boolean;
    showMobileNav?: boolean;
  } = $props();

  const allItems = $derived([...items, ...groups.flatMap((group) => group.items)]);

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

<nav class="sidebar-nav flex-1 overflow-y-auto p-2">
  {#each items as item (item.href)}
    {@const active = isActive(item.href, allItems)}
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- generic component with dynamic routes -->
    <a
      href={item.href}
      aria-current={active ? 'page' : undefined}
      class={['sidebar-item', active ? 'bg-surface' : '']}
    >
      <span class="sidebar-icon {item.icon}"></span>
      {item.label}
    </a>
  {/each}

  {#each groups as group (group.label)}
    <details class="group/nav mt-2" open>
      <summary
        class="flex min-h-9 cursor-pointer list-none items-center gap-2 rounded-md px-2 text-xs font-semibold tracking-wide text-muted uppercase transition-colors hover:bg-surface hover:text-text focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-action"
      >
        <span class="min-w-0 flex-1 truncate">{group.label}</span>
        <span
          class="iconify icon-[uil--angle-down] shrink-0 text-base transition-transform group-open/nav:rotate-180"
          aria-hidden="true"
        ></span>
      </summary>
      <div class="mt-1">
        {#each group.items as item (item.href)}
          {@const active = isActive(item.href, allItems)}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- generic component with dynamic routes -->
          <a
            href={item.href}
            aria-current={active ? 'page' : undefined}
            class={['sidebar-item', active ? 'bg-surface' : '']}
          >
            <span class="sidebar-icon {item.icon}"></span>
            {item.label}
          </a>
        {/each}
      </div>
    </details>
  {/each}
</nav>
