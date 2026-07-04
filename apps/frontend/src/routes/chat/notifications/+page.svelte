<script lang="ts">
  import { goto } from '$app/navigation';
  import { PaneHeader, EmptyState } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import * as m from '$lib/i18n/messages';
  import type { NotificationItem } from '$lib/state/server/notifications.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';

  const userSettings = getUserSettings();
  const activeLocale = $derived(getLocale());
  const appUi = getAppUiState();

  // Collect notification stores from all authenticated instances
  type ServerNotification = {
    serverId: string;
    serverName: string;
    serverHostname: string;
    notification: NotificationItem;
  };

  // Reactive: aggregate notifications from all authenticated instances
  let allNotifications = $derived.by(() => {
    const result: ServerNotification[] = [];

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;

      let hostname: string;
      try {
        hostname = new URL(instance.url).hostname;
      } catch {
        hostname = instance.url;
      }

      const store = stores.notifications;
      for (const notification of store.notifications) {
        result.push({
          serverId: instance.id,
          serverName: stores.serverInfo.name,
          serverHostname: hostname,
          notification
        });
      }
    }

    // Sort by creation time, newest first
    result.sort(
      (a, b) =>
        new Date(b.notification.createdAt).getTime() - new Date(a.notification.createdAt).getTime()
    );
    return result;
  });

  let loading = $state(true);
  // Fetch notifications from all authenticated instances on mount
  $effect(() => {
    fetchAll();
  });

  async function fetchAll() {
    loading = true;
    const fetches: Promise<void>[] = [];

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;
      fetches.push(stores.notifications.fetch());
    }

    await Promise.allSettled(fetches);
    loading = false;
  }

  function formatTime(timestamp: string): string {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / (1000 * 60));
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffMins < 1) return m['chat.notifications.time_now']();
    if (diffMins < 60) return m['chat.notifications.time_minutes']({ count: diffMins });
    if (diffHours < 24) return m['chat.notifications.time_hours']({ count: diffHours });
    if (diffDays < 7) return m['chat.notifications.time_days']({ count: diffDays });

    return formatDate(date, userSettings, activeLocale);
  }

  async function handleClick(item: ServerNotification) {
    const stores = serverRegistry.getStore(item.serverId);
    const store = stores.notifications;

    const target = notificationTarget(item.notification);
    prepareUiForNotificationTarget(appUi, item.serverId, target);
    if (target.eventId && target.roomId) {
      stores.pendingHighlights.set(target.roomId, target.threadRootId, target.eventId);
    }
    void store.dismiss(item.notification.id).then((dismissed) => {
      if (dismissed && target.roomId) {
        stores.rooms.decrementUnreadNotification(target.roomId);
        void stores.rooms.refreshNotificationCounts();
      }
    });

    const path = store.getCleanPath(item.serverId, item.notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }

  async function handleDismiss(e: Event, item: ServerNotification) {
    e.stopPropagation();
    const stores = serverRegistry.getStore(item.serverId);
    const target = notificationTarget(item.notification);
    const dismissed = await stores.notifications.dismiss(item.notification.id);
    if (dismissed && target.roomId) {
      stores.rooms.decrementUnreadNotification(target.roomId);
      void stores.rooms.refreshNotificationCounts();
    }
  }

  async function handleClearAll() {
    const clears: Promise<void>[] = [];
    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;
      const hadNotifications = stores.notifications.unreadNotificationCount > 0;
      clears.push(
        stores.notifications.dismissAll().then((dismissed) => {
          if (hadNotifications || dismissed > 0) {
            stores.rooms.clearAllUnreadNotifications();
            void stores.rooms.refreshNotificationCounts();
          }
        })
      );
    }
    await Promise.allSettled(clears);
  }
</script>

<div class="flex h-full w-full flex-col">
  <PaneHeader
    title={m['chat.notifications.title']()}
    subtitle={m['chat.notifications.subtitle']()}
    showMobileNav
  >
    {#snippet actions()}
      {#if allNotifications.length > 0}
        <Button variant="ghost" size="sm" onclick={handleClearAll}>
          {m['chat.notifications.clear_all']()}
        </Button>
      {/if}
    {/snippet}
  </PaneHeader>

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if loading && allNotifications.length === 0}
      <div class="p-6 text-muted">{m['common.loading']()}</div>
    {:else if allNotifications.length === 0}
      <EmptyState icon="uil--bell-slash" title={m['chat.notifications.empty_title']()}>
        {m['chat.notifications.empty_body']()}
      </EmptyState>
    {:else}
      <div class="flex flex-col">
        {#each allNotifications as item (item.notification.id)}
          {@const actor = item.notification.actor ?? null}
          {@const location = serverRegistry
            .getStore(item.serverId)
            .notifications.getLocationString(item.notification, item.serverName)}
          <div
            class="flex w-full cursor-pointer items-center gap-3 border-b border-border px-4 py-3 transition-colors hover:bg-surface-100"
            role="button"
            tabindex="0"
            data-testid="notification-item"
            onclick={() => handleClick(item)}
            onkeydown={(e) => e.key === 'Enter' && handleClick(item)}
          >
            {#if actor}
              <UserAvatar user={actor} size="md" />
            {/if}

            <div class="min-w-0 flex-1">
              <p class="truncate">{item.notification.summary}</p>
              <p class="text-sm text-muted">
                <span class="truncate">{item.serverHostname}</span>
                {#if location}
                  <span class="mx-1">•</span>
                  <span class="truncate">{location}</span>
                {/if}
                <span class="mx-1">•</span>
                {formatTime(item.notification.createdAt)}
              </p>
            </div>

            <button
              type="button"
              class="iconify icon-action uil--times"
              title={m['common.dismiss']()}
              onclick={(e) => handleDismiss(e, item)}
            ></button>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
