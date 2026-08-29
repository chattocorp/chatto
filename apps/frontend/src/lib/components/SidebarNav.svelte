<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { PaneHeader, ScrollFader } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';

  export type NavItem = { href: string; label: string; icon: string };
  export type NavGroup = {
    label: string;
    items: NavItem[];
    persistKey: string;
  };
  type GroupNavItem = NavItem & { id: string };

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
    backHref?: string;
    backLabel?: string;
    isActive?: (href: string, items: NavItem[]) => boolean;
    showMobileNav?: boolean;
  } = $props();

  const allItems = $derived([...items, ...groups.flatMap((group) => group.items)]);
  const normalizedGroups = $derived(
    groups.map((group) => ({
      ...group,
      items: group.items.map((item) => ({ ...item, id: item.href }))
    }))
  );

  function defaultIsActive(href: string, items: NavItem[]): boolean {
    const pathname = page.url.pathname;
    if (pathname === href) return true;
    if (!pathname.startsWith(`${href}/`)) return false;

    // A parent route can represent a section, but only its most-specific
    // matching navigation item is the current page.
    return !items.some(
      (item) =>
        item.href.length > href.length &&
        (pathname === item.href || pathname.startsWith(`${item.href}/`))
    );
  }
</script>

{#snippet groupedNavItem(item: GroupNavItem)}
  {@const active = isActive(item.href, allItems)}
  <a
    href={resolve(item.href as '/')}
    aria-current={active ? 'page' : undefined}
    class="sidebar-item"
  >
    <span class="sidebar-icon {item.icon}"></span>
    {item.label}
  </a>
{/snippet}

<PaneHeader {title} {subtitle} {backHref} {backLabel} {showMobileNav} />

<ScrollFader top bottom>
  {#if items.length > 0}
    <nav class="sidebar-nav p-2">
      {#each items as item (item.href)}
        {@const active = isActive(item.href, allItems)}
        <a
          href={resolve(item.href as '/')}
          aria-current={active ? 'page' : undefined}
          class="sidebar-item"
        >
          <span class="sidebar-icon {item.icon}"></span>
          {item.label}
        </a>
      {/each}
    </nav>
  {/if}

  {#if normalizedGroups.length > 0}
    <nav>
      {#each normalizedGroups as group, index (group.persistKey)}
        <RoomGroupSection
          label={group.label}
          items={group.items}
          item={groupedNavItem}
          persistKey={group.persistKey}
          separated={index > 0 || items.length > 0}
        />
      {/each}
    </nav>
  {/if}
</ScrollFader>
