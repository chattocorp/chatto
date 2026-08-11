<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { EmptyState, PaneHeader, SegmentedControl } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import {
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
  const viewOptions = $derived([
    { value: NotificationView.INBOX, label: m('chat.notifications.inbox') },
    { value: NotificationView.DONE, label: m('chat.notifications.done') }
  ]);

  $effect(() => {
    void notificationViewInvalidations;
    void loadView(view);
  });

  // Done is a fetched view rather than a realtime payload. Reconcile it at its
  // own earliest expiry as well as on live invalidations above.
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

  function occurrenceReasonLabel(reasons: NotificationReason[]): string {
    if (reasons.includes(NotificationReason.DIRECT_MESSAGE)) {
      return m('settings.notifications.policy.reason.direct_message');
    }
    if (reasons.includes(NotificationReason.REACTION)) {
      return m('settings.notifications.policy.reason.reaction');
    }
    if (reasons.includes(NotificationReason.REPLY)) {
      return m('settings.notifications.policy.reason.reply');
    }
    if (reasons.includes(NotificationReason.DIRECT_MENTION)) {
      return m('settings.notifications.policy.reason.direct_mention');
    }
    if (reasons.includes(NotificationReason.ROLE_MENTION)) {
      return m('settings.notifications.policy.reason.role_mention');
    }
    if (reasons.includes(NotificationReason.HERE)) {
      return m('settings.notifications.policy.reason.here');
    }
    if (reasons.includes(NotificationReason.ALL)) {
      return m('settings.notifications.policy.reason.all');
    }
    if (reasons.includes(NotificationReason.FOLLOWED_THREAD)) {
      return m('settings.notifications.policy.reason.followed_thread');
    }
    if (reasons.includes(NotificationReason.FOLLOWED_ROOM)) {
      return m('settings.notifications.policy.reason.followed_room');
    }
    return m('settings.notifications.policy.reason.activity');
  }

  async function openGroup(item: ServerGroup) {
    const occurrence = item.group.openTarget;
    if (!occurrence) return;
    const stores = serverRegistry.getStore(item.serverId);
    const roomId = occurrence.room?.id ?? null;
    prepareUiForNotificationTarget(appUi, item.serverId, { roomId });
    if (roomId && occurrence.eventId) {
      stores.pendingHighlights.set(roomId, occurrence.threadRootId, occurrence.eventId);
    }
    await navigateToDestination(item.serverId, occurrence);
    if (
      view === NotificationView.INBOX &&
      occurrence.inboxState === NotificationInboxState.UNREAD
    ) {
      await stores.notifications.markOccurrenceRead(occurrence.id);
    }
  }

  async function mutate(item: ServerGroup, action: 'done' | 'restore' | 'delete') {
    const store = serverRegistry.getStore(item.serverId).notifications;
    if (action === 'done') await store.moveGroupToDone(item.group.id, view);
    if (action === 'restore') await store.restoreGroupToInbox(item.group.id, view);
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

  <div class="border-b border-border px-4 py-2">
    <SegmentedControl
      label={m('chat.notifications.title')}
      options={viewOptions}
      value={view}
      onchange={selectView}
    />
  </div>

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if loading && groups.length === 0}
      <div class="p-6 text-muted">{m('common.loading')}</div>
    {:else if groups.length === 0}
      <EmptyState icon="icon-[uil--bell-slash]" title={m('chat.notifications.empty_title')}>
        {m('chat.notifications.empty_body')}
      </EmptyState>
    {:else}
      <div class="selectable-list">
        {#each groups as item (`${item.serverId}:${item.group.id}`)}
          {@const occurrence = item.group.openTarget}
          {@const actor = occurrence?.actor ?? null}
          <div
            class={[
              'group flex w-full cursor-pointer items-center gap-3 selectable-list-item px-3 py-2.5',
              item.group.unread && view === NotificationView.INBOX && 'bg-action/5'
            ]}
            data-testid="notification-group"
          >
            <button
              type="button"
              class="flex min-w-0 flex-1 cursor-pointer items-center gap-3 rounded-md text-start focus-visible:outline-2 focus-visible:outline-action"
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
                <span class="block truncate font-medium" dir="auto">
                  {#if actor}<bdi dir="auto">{actor.displayName}</bdi><span aria-hidden="true">
                      ·
                    </span>{/if}{occurrence
                    ? occurrenceReasonLabel(occurrence.reasons)
                    : m('chat.notifications.activity')}
                </span>
                <span class="block truncate text-sm text-muted">
                  {item.serverHostname}
                  {#if occurrence?.room?.name}
                    · <bdi dir="auto">#{occurrence.room.name}</bdi>{/if}
                  · {item.group.occurrenceCount} · {formatTime(
                    item.group.latestAt,
                    item.timeFormatSettings
                  )}
                </span>
              </span>
            </button>
            <div class="flex shrink-0 items-center gap-2">
              {#if view === NotificationView.INBOX}
                <Button
                  variant="secondary"
                  size="sm"
                  label={m('chat.notifications.mark_done')}
                  title={m('chat.notifications.mark_done')}
                  onclick={() => mutate(item, 'done')}
                >
                  <span class="iconify icon-[uil--check] text-base" aria-hidden="true"></span>
                </Button>
              {:else if view === NotificationView.DONE}
                <Button
                  variant="secondary"
                  size="sm"
                  label={m('chat.notifications.restore')}
                  title={m('chat.notifications.restore')}
                  onclick={() => mutate(item, 'restore')}
                >
                  <span class="iconify icon-[uil--redo] text-base" aria-hidden="true"></span>
                </Button>
              {/if}
              <Button
                variant="danger-secondary"
                size="sm"
                label={m('common.delete')}
                title={m('common.delete')}
                onclick={() => mutate(item, 'delete')}
              >
                <span class="iconify icon-[uil--trash-alt] text-base" aria-hidden="true"></span>
              </Button>
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
