<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NotificationBadge from './NotificationBadge.svelte';
  import SidebarNavigationItem from './SidebarNavigationItem.svelte';
  import UnreadDot from './UnreadDot.svelte';

  const { Story } = defineMeta({
    title: 'UI/SidebarNavigationItem',
    component: SidebarNavigationItem,
    tags: ['autodocs']
  });
</script>

{#snippet archiveAction()}
  <button type="button" class="mini-icon-action" aria-label="Archive conversation">
    <span class="iconify icon-[uil--archive] text-base" aria-hidden="true"></span>
  </button>
{/snippet}

{#snippet notificationStatus()}
  <button
    type="button"
    class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
    aria-label="Go to 3 direct message notifications"
  >
    <NotificationBadge count={3} color="ambient" />
  </button>
{/snippet}

{#snippet unreadStatus()}
  <UnreadDot color="neutral" />
{/snippet}

<Story name="Status replaced by hover action" asChild>
  <div class="flex w-72 flex-col gap-1 bg-background p-3">
    <SidebarNavigationItem
      href="#sidebar-navigation-notification"
      status={notificationStatus}
      hoverAction={archiveAction}
    >
      <span
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized"
        >R</span
      >
      <span class="min-w-0 flex-1 truncate">River</span>
    </SidebarNavigationItem>

    <SidebarNavigationItem
      href="#sidebar-navigation-unread"
      status={unreadStatus}
      hoverAction={archiveAction}
    >
      <span
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized"
        >S</span
      >
      <span class="min-w-0 flex-1 truncate">Sage</span>
    </SidebarNavigationItem>

    <SidebarNavigationItem href="#sidebar-navigation-quiet" hoverAction={archiveAction}>
      <span
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized"
        >T</span
      >
      <span class="min-w-0 flex-1 truncate">Teal</span>
    </SidebarNavigationItem>
  </div>
</Story>
