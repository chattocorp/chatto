<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { PaneHeader, EmptyState } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import {
    NotificationDeliveryIntensity,
    NotificationInboxState,
    NotificationReason,
    NotificationView,
    type NotificationGroupItem,
    type NotificationOccurrenceItem
  } from '$lib/api-client/notifications';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import {
    formatDate,
    timeFormatSettingsFor,
    type TimeFormatSettings
  } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';

  type ServerGroup = {
    serverId: string;
    serverHostname: string;
    timeFormatSettings: TimeFormatSettings;
    group: NotificationGroupItem;
  };

  const activeLocale = $derived(getLocale());
  const appUi = getAppUiState();
  let view = $state(NotificationView.INBOX);
  let groups = $state.raw<ServerGroup[]>([]);
  let loading = $state(true);
  let loadingMore = $state(false);
  let pageError = $state(false);
  let loadGeneration = 0;
  let pagination = $state.raw<Record<string, { offset: number; hasMore: boolean }>>({});
  const hasMore = $derived(Object.values(pagination).some((state) => state.hasMore));
  const notificationViewInvalidations = $derived(
    serverRegistry.servers
      .map((instance) => serverRegistry.getStore(instance.id).notifications.viewInvalidationVersion)
      .join(':')
  );

  $effect(() => {
    void notificationViewInvalidations;
    void loadView(view);
  });

  // Done and Saved are fetched views rather than realtime payloads. Reconcile
  // them at their own earliest expiry as well as on live invalidations above.
  $effect(() => {
    if (groups.length === 0) return;
    const expiry = groups.reduce<number | null>((earliest, item) => {
      if (!item.group.nextExpiryAt) return earliest;
      const value = new Date(item.group.nextExpiryAt).getTime() + 50;
      return earliest === null || value < earliest ? value : earliest;
    }, null);
    if (expiry === null) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const schedule = () => {
      const remaining = expiry - Date.now();
      if (remaining <= 0) {
        void loadView(view);
        return;
      }
      timer = setTimeout(schedule, Math.min(remaining, 2_147_483_647));
    };
    schedule();
    return () => {
      if (timer) clearTimeout(timer);
    };
  });

  async function loadView(nextView: NotificationView) {
    const generation = ++loadGeneration;
    loading = true;
    loadingMore = false;
    const results = await Promise.allSettled(
      serverRegistry.servers.map(async (instance) => {
        const stores = serverRegistry.getStore(instance.id);
        if (!stores.isAuthenticated) return null;
        const page = await stores.notifications.fetchView(nextView);
        let hostname: string;
        try {
          hostname = new URL(instance.url).hostname;
        } catch {
          hostname = instance.url;
        }
        return {
          serverId: instance.id,
          page,
          groups: page.groups.map((group): ServerGroup => ({
            serverId: instance.id,
            serverHostname: hostname,
            timeFormatSettings: timeFormatSettingsFor(stores.currentUser.user?.settings),
            group
          }))
        };
      })
    );
    if (nextView !== view || generation !== loadGeneration) return;
    groups = results
      .flatMap((result) =>
        result.status === 'fulfilled' && result.value ? result.value.groups : []
      )
      .sort((a, b) => b.group.latestAt.localeCompare(a.group.latestAt));
    pagination = Object.fromEntries(
      results.flatMap((result) => {
        if (result.status !== 'fulfilled' || !result.value) return [];
        return [
          [
            result.value.serverId,
            {
              offset: result.value.page.groups.length,
              hasMore: result.value.page.hasMore
            }
          ] as const
        ];
      })
    );
    pageError = results.some((result) => result.status === 'rejected');
    loading = false;
  }

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;
    const generation = loadGeneration;
    const selectedView = view;
    loadingMore = true;
    const pending = Object.entries(pagination).filter(([, state]) => state.hasMore);
    const results = await Promise.allSettled(
      pending.map(async ([serverId, state]) => {
        const stores = serverRegistry.getStore(serverId);
        const page = await stores.notifications.fetchView(selectedView, state.offset);
        let hostname: string;
        const instance = serverRegistry.servers.find(({ id }) => id === serverId);
        try {
          hostname = new URL(instance?.url ?? '').hostname;
        } catch {
          hostname = instance?.url ?? serverId;
        }
        return {
          serverId,
          page,
          groups: page.groups.map((group): ServerGroup => ({
            serverId,
            serverHostname: hostname,
            timeFormatSettings: timeFormatSettingsFor(stores.currentUser.user?.settings),
            group
          }))
        };
      })
    );
    if (generation !== loadGeneration || selectedView !== view) {
      loadingMore = false;
      return;
    }
    groups = [
      ...groups,
      ...results.flatMap((result) => (result.status === 'fulfilled' ? result.value.groups : []))
    ].sort((a, b) => b.group.latestAt.localeCompare(a.group.latestAt));
    pagination = Object.fromEntries(
      Object.entries(pagination).map(([serverId, state]) => {
        const result = results.find(
          (candidate) => candidate.status === 'fulfilled' && candidate.value.serverId === serverId
        );
        if (!result || result.status !== 'fulfilled') return [serverId, state];
        return [
          serverId,
          {
            offset: state.offset + result.value.page.groups.length,
            hasMore: result.value.page.hasMore && result.value.page.groups.length > 0
          }
        ];
      })
    );
    pageError = results.some((result) => result.status === 'rejected');
    loadingMore = false;
  }

  const loadMoreWhenVisible = useLoadMoreWhenVisible({
    getCursor: () => (hasMore ? JSON.stringify(pagination) : null),
    loadMore,
    hasError: () => pageError
  });

  function selectView(nextView: NotificationView) {
    if (view === nextView) return;
    view = nextView;
  }

  function formatTime(timestamp: string, settings: TimeFormatSettings): string {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMins = Math.floor((now.getTime() - date.getTime()) / 60_000);
    const diffHours = Math.floor(diffMins / 60);
    const diffDays = Math.floor(diffHours / 24);
    if (diffMins < 1) return m('chat.notifications.time_now');
    if (diffMins < 60) return m('chat.notifications.time_minutes', { count: diffMins });
    if (diffHours < 24) return m('chat.notifications.time_hours', { count: diffHours });
    if (diffDays < 7) return m('chat.notifications.time_days', { count: diffDays });
    return formatDate(date, settings, activeLocale);
  }

  async function navigateToDestination(
    serverId: string,
    occurrence: NotificationOccurrenceItem
  ): Promise<void> {
    const serverIdSegment = serverIdToSegment(serverId);
    const roomId = occurrence.room?.id;
    if (!roomId) {
      await goto(resolve('/chat/[serverId]', { serverId: serverIdSegment }));
      return;
    }
    if (occurrence.threadRootId) {
      await goto(
        resolve('/chat/[serverId]/[roomId]/[threadId]', {
          serverId: serverIdSegment,
          roomId,
          threadId: occurrence.threadRootId
        })
      );
      return;
    }
    await goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverIdSegment, roomId }));
  }

  async function openGroup(item: ServerGroup) {
    const occurrence = item.group.openTarget;
    if (!occurrence) return;
    const stores = serverRegistry.getStore(item.serverId);
    if (view === NotificationView.INBOX && item.group.unread) {
      await stores.notifications.updateGroup(item.group.id, view, {
        inboxState: NotificationInboxState.READ
      });
    }
    const roomId = occurrence.room?.id ?? null;
    prepareUiForNotificationTarget(appUi, item.serverId, { roomId });
    if (roomId && occurrence.eventId) {
      stores.pendingHighlights.set(roomId, occurrence.threadRootId, occurrence.eventId);
    }
    await navigateToDestination(item.serverId, occurrence);
  }

  async function mutate(
    item: ServerGroup,
    action: 'done' | 'restore' | 'save' | 'unsubscribe' | 'delete'
  ) {
    const store = serverRegistry.getStore(item.serverId).notifications;
    if (action === 'done') await store.moveGroupToDone(item.group.id, view);
    if (action === 'restore') await store.restoreGroupToInbox(item.group.id, view);
    if (action === 'save') {
      const allSaved =
        item.group.allSaved ?? item.group.occurrences.every((occurrence) => occurrence.saved);
      await store.setGroupSaved(item.group.id, view, !allSaved);
    }
    if (action === 'unsubscribe') await store.unsubscribeGroup(item.group.id, view);
    if (action === 'delete') await store.deleteGroup(item.group.id, view);
    await loadView(view);
  }
</script>

<div class="flex h-full w-full flex-col">
  <PaneHeader
    title={m('chat.notifications.title')}
    subtitle={m('chat.notifications.subtitle')}
    showMobileNav
  />

  <div class="flex gap-1 border-b border-border px-4 py-2" role="tablist">
    <Button
      variant={view === NotificationView.INBOX ? 'action' : 'ghost'}
      size="sm"
      onclick={() => selectView(NotificationView.INBOX)}
    >
      {m('chat.notifications.inbox')}
    </Button>
    <Button
      variant={view === NotificationView.DONE ? 'action' : 'ghost'}
      size="sm"
      onclick={() => selectView(NotificationView.DONE)}
    >
      {m('chat.notifications.done')}
    </Button>
    <Button
      variant={view === NotificationView.SAVED ? 'action' : 'ghost'}
      size="sm"
      onclick={() => selectView(NotificationView.SAVED)}
    >
      {m('chat.notifications.saved')}
    </Button>
  </div>

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if loading && groups.length === 0}
      <div class="p-6 text-muted">{m('common.loading')}</div>
    {:else if groups.length === 0}
      <EmptyState icon="icon-[uil--bell-slash]" title={m('chat.notifications.empty_title')}>
        {m('chat.notifications.empty_body')}
      </EmptyState>
    {:else}
      <div class="flex flex-col">
        {#each groups as item (`${item.serverId}:${item.group.id}`)}
          {@const occurrence = item.group.openTarget}
          {@const actor = occurrence?.actor ?? null}
          {@const allSaved =
            item.group.allSaved ?? item.group.occurrences.every((member) => member.saved)}
          {@const canUnsubscribe =
            item.group.canUnsubscribe ??
            item.group.occurrences.some((member) =>
              member.reasonMatches.some(
                (match) =>
                  match.intensity > NotificationDeliveryIntensity.OFF &&
                  (match.reason === NotificationReason.FOLLOWED_THREAD ||
                    match.reason === NotificationReason.FOLLOWED_ROOM)
              )
            )}
          <div
            class={[
              'group flex w-full items-center gap-3 border-b border-border px-4 py-3 transition-colors hover:bg-surface',
              item.group.unread && view === NotificationView.INBOX && 'bg-action/5'
            ]}
            data-testid="notification-group"
          >
            <button
              type="button"
              class="flex min-w-0 flex-1 items-center gap-3 text-left"
              onclick={() => openGroup(item)}
            >
              {#if actor}<UserAvatar user={actor} size="md" />{/if}
              {#if item.group.unread && view === NotificationView.INBOX}
                <span
                  class="size-2 shrink-0 rounded-full bg-action"
                  aria-label={m('chat.notifications.unread')}
                ></span>
              {/if}
              <span class="min-w-0 flex-1">
                <span class="block truncate font-medium"
                  >{occurrence?.summary ?? m('chat.notifications.activity')}</span
                >
                <span class="block truncate text-sm text-muted">
                  {item.serverHostname}
                  {#if occurrence?.room?.name}
                    · #{occurrence.room.name}{/if}
                  · {item.group.occurrenceCount} · {formatTime(
                    item.group.latestAt,
                    item.timeFormatSettings
                  )}
                </span>
              </span>
            </button>
            <div class="flex shrink-0 items-center gap-1">
              <button
                type="button"
                class={[
                  'iconify icon-action',
                  allSaved ? 'icon-[uil--bookmark]' : 'icon-[uil--bookmark-full]'
                ]}
                title={allSaved ? m('chat.notifications.unsave') : m('chat.notifications.save')}
                onclick={() => mutate(item, 'save')}
              ></button>
              {#if view === NotificationView.INBOX}
                {#if canUnsubscribe}
                  <button
                    type="button"
                    class="iconify icon-action icon-[uil--bell-slash]"
                    title={m('chat.notifications.unsubscribe')}
                    onclick={() => mutate(item, 'unsubscribe')}
                  ></button>
                {/if}
                <button
                  type="button"
                  class="iconify icon-action icon-[uil--check]"
                  title={m('chat.notifications.mark_done')}
                  onclick={() => mutate(item, 'done')}
                ></button>
              {:else if view === NotificationView.DONE}
                <button
                  type="button"
                  class="iconify icon-action icon-[uil--redo]"
                  title={m('chat.notifications.restore')}
                  onclick={() => mutate(item, 'restore')}
                ></button>
              {/if}
              <button
                type="button"
                class="iconify icon-action icon-[uil--trash-alt]"
                title={m('common.delete')}
                onclick={() => mutate(item, 'delete')}
              ></button>
            </div>
          </div>
        {/each}
        {#if hasMore}
          <div class="flex min-h-14 justify-center p-4 text-muted" {@attach loadMoreWhenVisible}>
            {#if loadingMore}{m('common.loading')}{/if}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
