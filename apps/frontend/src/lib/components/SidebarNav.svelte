<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { PaneHeader, ScrollFader } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';

  export type NavItem = {
    href: string;
    label: string;
    icon: string;
    /** Open the Shield Mode picker for this destination. */
    requestAccess?: () => void;
    /** The viewer can open this page because Shield Mode is active. */
    shieldActive?: boolean;
  };
  /** A command in a navigation group, such as Sign out. It is not a page. */
  export type NavAction = {
    id: string;
    label: string;
    icon: string;
    onclick: () => void;
    disabled?: boolean;
  };
  export type NavGroup = {
    label: string;
    items: NavItem[];
    /** Commands after the group's links. */
    actions?: NavAction[];
    persistKey: string;
  };
  type GroupNavItem = NavItem & { id: string };

  let {
    title,
    subtitle,
    items = [],
    groups = [],
    backHref,
    onBack,
    backLabel = m('ui.sidebar_nav.back_to_chat'),
    isActive = defaultIsActive
  }: {
    title: string;
    subtitle?: string;
    items?: NavItem[];
    groups?: NavGroup[];
    backHref?: string;
    /** Run a back action when the destination depends on navigation history. */
    onBack?: () => void;
    backLabel?: string;
    isActive?: (href: string, items: NavItem[]) => boolean;
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

<!-- Labels can wrap. The icon box has the height of one text line, so the
     icon stays centred on the first line. -->
{#snippet iconLabel(icon: string, label: string)}
  <span aria-hidden="true" class="flex h-lh shrink-0 items-center">
    <span class="sidebar-icon {icon}"></span>
  </span>
  <span class="min-w-0 text-start">{label}</span>
{/snippet}

{#snippet groupedNavItem(item: GroupNavItem)}
  {@const active = isActive(item.href, allItems)}
  {#if item.requestAccess}
    <button
      type="button"
      class="sidebar-item items-start text-muted opacity-75 hover:opacity-100 focus-visible:opacity-100"
      aria-haspopup="dialog"
      title={m('chat.privileged_mode.enable')}
      onclick={item.requestAccess}
    >
      {@render iconLabel(item.icon, item.label)}
      <span class="iconify ms-auto icon-[uil--shield] shrink-0 self-center" aria-hidden="true"
      ></span>
    </button>
  {:else}
    <a
      href={resolve(item.href as '/')}
      title={item.shieldActive ? m('settings.overview.shield_active') : undefined}
      aria-current={active ? 'page' : undefined}
      class="sidebar-item items-start"
    >
      {@render iconLabel(item.icon, item.label)}
      {#if item.shieldActive}
        <span
          class="iconify ms-auto icon-[uil--shield-check] shrink-0 self-center text-warning"
          aria-hidden="true"
        ></span>
      {/if}
    </a>
  {/if}
{/snippet}

<PaneHeader {title} {subtitle} {backHref} {onBack} {backLabel} />

<ScrollFader top bottom>
  <nav aria-label={title}>
    {#if items.length > 0}
      <div class="sidebar-nav p-2">
        {#each items as item (item.href)}
          {@const active = isActive(item.href, allItems)}
          <a
            href={resolve(item.href as '/')}
            aria-current={active ? 'page' : undefined}
            class="sidebar-item items-start"
          >
            {@render iconLabel(item.icon, item.label)}
          </a>
        {/each}
      </div>
    {/if}

    {#if normalizedGroups.length > 0}
      {#each normalizedGroups as group, index (group.persistKey)}
        <RoomGroupSection
          label={group.label}
          items={group.items}
          item={groupedNavItem}
          persistKey={group.persistKey}
          separated={index > 0 || items.length > 0}
        >
          {#snippet footer()}
            {#if group.actions?.length}
              <div class="mt-0.5 flex flex-col gap-0.5">
                {#each group.actions as action (action.id)}
                  <button
                    type="button"
                    class="sidebar-item items-start disabled:cursor-not-allowed disabled:opacity-50"
                    disabled={action.disabled}
                    data-testid="sidebar-nav-action-{action.id}"
                    onclick={action.onclick}
                  >
                    {@render iconLabel(action.icon, action.label)}
                  </button>
                {/each}
              </div>
            {/if}
          {/snippet}
        </RoomGroupSection>
      {/each}
    {/if}
  </nav>
</ScrollFader>
