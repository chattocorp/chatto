<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { EmptyState, PaneHeader } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import {
    groupNotificationOccurrences,
    NotificationReason,
    type NotificationActor,
    type NotificationGroupItem,
    type NotificationOccurrenceItem
  } from '$lib/api-client/notifications';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import {
    fileDateGroup,
    formatDate,
    formatMonthYear,
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

  type PaginationSource = {
    serverId: string;
    offset: number;
    hasMore: boolean;
  };

  type NotificationDateSection = {
    key: string;
    label: string;
    items: ServerGroup[];
  };

  const activeLocale = $derived(getLocale());
  const appUi = getAppUiState();
  let groups = $state.raw<ServerGroup[]>([]);
  let loading = $state(true);
  let loadingMore = $state(false);
  let pageError = $state(false);
  let dismissingAll = $state(false);
  let loadGeneration = 0;
  let pagination = $state.raw<PaginationSource[]>([]);
  const pendingMutationKeys = new SvelteSet<string>();
  const hasPendingMutation = $derived(pendingMutationKeys.size > 0);
  const hasMore = $derived(pagination.some((source) => source.hasMore));
  const showServerHostname = $derived(
    serverRegistry.servers.filter(
      (instance) => serverRegistry.getStore(instance.id).isAuthenticated
    ).length > 1
  );
  const visibleGroups = $derived.by(() => {
    const sorted = [...groups].sort(compareGroups);
    const activeBoundaries = pagination
      .filter((source) => source.hasMore)
      .map(
        (source) =>
          groups.filter((item) => item.serverId === source.serverId).at(-1)?.group.latestAt
      )
      .filter((timestamp): timestamp is string => Boolean(timestamp));
    if (activeBoundaries.length === 0) return sorted;
    const newestUnloadedBoundary = activeBoundaries.sort().at(-1);
    return sorted.filter((item) => item.group.latestAt >= newestUnloadedBoundary!);
  });
  const dateSections = $derived.by(() => groupNotificationsByDate(visibleGroups));
  const notificationViewInvalidations = $derived(
    serverRegistry.servers
      .map((instance) => serverRegistry.getStore(instance.id).notifications.viewInvalidationVersion)
      .join(':')
  );
  $effect(() => {
    void notificationViewInvalidations;
    // Initial projection hydration and one logical mutation can emit several
    // adjacent invalidations. Coalesce them into one authoritative list read
    // per authenticated server.
    const timer = setTimeout(() => void loadNotifications(), 50);
    return () => clearTimeout(timer);
  });

  // Reconcile the list at its earliest expiry as well as on live invalidations.
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
        void loadNotifications();
        return;
      }
      timer = setTimeout(schedule, Math.min(remaining, 2_147_483_647));
    };
    schedule();
    return () => {
      if (timer) clearTimeout(timer);
    };
  });

  async function loadNotifications() {
    const generation = ++loadGeneration;
    loading = true;
    loadingMore = false;
    const requests = serverRegistry.servers.flatMap((instance) => {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) return [];
      return [
        {
          serverId: instance.id,
          request: (async () => {
            const page = await stores.notifications.fetchPage();
            let hostname: string;
            try {
              hostname = new URL(instance.url).hostname;
            } catch {
              hostname = instance.url;
            }
            return {
              serverId: instance.id,
              page,
              groups: groupNotificationOccurrences(page.occurrences).map((group): ServerGroup => ({
                serverId: instance.id,
                serverHostname: hostname,
                timeFormatSettings: timeFormatSettingsFor(stores.currentUser.user?.settings),
                group
              }))
            };
          })()
        }
      ];
    });
    const results = await Promise.allSettled(requests.map(({ request }) => request));
    if (generation !== loadGeneration) return;
    groups = results
      .flatMap((result) => (result.status === 'fulfilled' ? result.value.groups : []))
      .sort(compareGroups);
    pagination = results.flatMap((result, index): PaginationSource[] => {
      if (result.status !== 'fulfilled') {
        return [
          {
            serverId: requests[index].serverId,
            offset: 0,
            hasMore: true
          }
        ];
      }
      return [
        {
          serverId: result.value.serverId,
          offset: result.value.page.occurrences.length,
          hasMore: result.value.page.hasMore
        }
      ];
    });
    pageError = results.some((result) => result.status === 'rejected');
    loading = false;
  }

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;
    const generation = loadGeneration;
    loadingMore = true;
    pageError = false;
    const pending = pagination.filter((source) => source.hasMore);
    const results = await Promise.allSettled(
      pending.map(async (source) => {
        const stores = serverRegistry.getStore(source.serverId);
        const page = await stores.notifications.fetchPage(source.offset);
        let hostname: string;
        const instance = serverRegistry.servers.find(({ id }) => id === source.serverId);
        try {
          hostname = new URL(instance?.url ?? '').hostname;
        } catch {
          hostname = instance?.url ?? source.serverId;
        }
        return {
          serverId: source.serverId,
          page,
          groups: groupNotificationOccurrences(page.occurrences).map((group): ServerGroup => ({
            serverId: source.serverId,
            serverHostname: hostname,
            timeFormatSettings: timeFormatSettingsFor(stores.currentUser.user?.settings),
            group
          }))
        };
      })
    );
    if (generation !== loadGeneration) {
      loadingMore = false;
      return;
    }
    groups = mergeServerGroups(
      groups,
      results.flatMap((result) => (result.status === 'fulfilled' ? result.value.groups : []))
    );
    pagination = pagination.map((source) => {
      const result = results.find(
        (candidate) =>
          candidate.status === 'fulfilled' && candidate.value.serverId === source.serverId
      );
      if (!result || result.status !== 'fulfilled') return source;
      return {
        ...source,
        offset: source.offset + result.value.page.occurrences.length,
        hasMore: result.value.page.hasMore && result.value.page.occurrences.length > 0
      };
    });
    pageError = results.some((result) => result.status === 'rejected');
    loadingMore = false;
  }

  const loadMoreWhenVisible = useLoadMoreWhenVisible({
    getCursor: () => (hasMore ? JSON.stringify(pagination) : null),
    loadMore,
    hasError: () => pageError
  });

  function mutationKey(item: ServerGroup): string {
    return `${item.serverId}:${item.group.id}`;
  }

  function rowKey(item: ServerGroup): string {
    return `${item.serverId}:${item.group.id}`;
  }

  function compareGroups(a: ServerGroup, b: ServerGroup): number {
    const byTime = b.group.latestAt.localeCompare(a.group.latestAt);
    if (byTime !== 0) return byTime;
    return mutationKey(a).localeCompare(mutationKey(b));
  }

  function mergeServerGroups(current: ServerGroup[], incoming: ServerGroup[]): ServerGroup[] {
    const metadata = new SvelteMap<string, Omit<ServerGroup, 'group'>>();
    const occurrences = new SvelteMap<string, NotificationOccurrenceItem[]>();
    for (const item of [...current, ...incoming]) {
      metadata.set(item.serverId, item);
      const existing = occurrences.get(item.serverId) ?? [];
      occurrences.set(item.serverId, [
        ...existing,
        ...item.group.occurrences.filter(
          (occurrence) => !existing.some((candidate) => candidate.id === occurrence.id)
        )
      ]);
    }
    return [...occurrences.entries()]
      .flatMap(([serverId, items]) => {
        const server = metadata.get(serverId)!;
        return groupNotificationOccurrences(items).map((group) => ({ ...server, group }));
      })
      .sort(compareGroups);
  }

  function groupNotificationsByDate(items: ServerGroup[]): NotificationDateSection[] {
    const sections: NotificationDateSection[] = [];
    const now = new Date();
    for (const item of items) {
      const dateGroup = fileDateGroup(
        item.group.latestAt,
        item.timeFormatSettings,
        now,
        activeLocale
      );
      const label =
        dateGroup.key === 'this-month'
          ? formatMonthYear(item.group.latestAt, item.timeFormatSettings, activeLocale)
          : dateGroup.label;
      const key = dateGroup.key === 'this-month' ? `this-month:${label}` : dateGroup.key;
      let section = sections.find((candidate) => candidate.key === key);
      if (!section) {
        section = { key, label, items: [] };
        sections.push(section);
      }
      section.items.push(item);
    }
    return sections;
  }

  function setMutationPending(key: string, pending: boolean): void {
    if (pending) pendingMutationKeys.add(key);
    else pendingMutationKeys.delete(key);
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

  function occurrenceSummary(group: NotificationGroupItem): string {
    const occurrence = group.openTarget;
    if (!occurrence) return m('chat.notifications.activity');
    const actor = occurrence.actor?.displayName;
    if (!actor) return m('chat.notifications.activity');
    const reasons = occurrence.reasons;
    if (reasons.includes(NotificationReason.DIRECT_MESSAGE)) {
      return m('chat.notifications.summary.direct_message', { actor });
    }
    if (reasons.includes(NotificationReason.REACTION)) {
      const emojis = [
        ...new Set(group.occurrences.map((item) => item.reactionEmoji).filter(Boolean))
      ];
      const prefix = emojis.join(' ');
      const summary = m('chat.notifications.summary.reaction', { actor });
      return prefix ? `${prefix} ${summary}` : summary;
    }
    if (reasons.includes(NotificationReason.REPLY)) {
      return m('chat.notifications.summary.reply', { actor });
    }
    if (
      reasons.includes(NotificationReason.DIRECT_MENTION) ||
      reasons.includes(NotificationReason.ROLE_MENTION) ||
      reasons.includes(NotificationReason.HERE) ||
      reasons.includes(NotificationReason.ALL)
    ) {
      return m('chat.notifications.summary.mention', { actor });
    }
    if (reasons.includes(NotificationReason.FOLLOWED_THREAD)) {
      return m('chat.notifications.summary.followed_thread', { actor });
    }
    if (reasons.includes(NotificationReason.FOLLOWED_ROOM)) {
      return m('chat.notifications.summary.new_message', { actor });
    }
    return m('chat.notifications.summary.activity', { actor });
  }

  function notificationActors(group: NotificationGroupItem): NotificationActor[] {
    const actors = new SvelteMap<string, NotificationActor>();
    for (const occurrence of group.occurrences) {
      if (occurrence.actor) actors.set(occurrence.actor.id, occurrence.actor);
    }
    return [...actors.values()].slice(0, 3);
  }

  async function openGroup(item: ServerGroup) {
    const key = mutationKey(item);
    if (pendingMutationKeys.has(key)) return;
    const occurrence = item.group.openTarget;
    if (!occurrence) return;
    setMutationPending(key, true);
    try {
      const stores = serverRegistry.getStore(item.serverId);
      const roomId = occurrence.room?.id ?? null;
      prepareUiForNotificationTarget(appUi, item.serverId, { roomId });
      if (roomId && occurrence.eventId) {
        stores.pendingHighlights.set(
          roomId,
          occurrence.threadRootId,
          occurrence.eventId,
          occurrence.unread ? occurrence.id : null
        );
      }
      await navigateToDestination(item.serverId, occurrence);
    } catch (error) {
      console.error('Failed to open notification:', error);
      toast.error(m('common.error.network'));
    } finally {
      setMutationPending(key, false);
    }
  }

  async function dismiss(item: ServerGroup) {
    const key = mutationKey(item);
    if (pendingMutationKeys.has(key)) return;
    setMutationPending(key, true);
    const store = serverRegistry.getStore(item.serverId).notifications;
    const removed = groups.find((candidate) => rowKey(candidate) === rowKey(item));
    groups = groups.filter((candidate) => rowKey(candidate) !== rowKey(item));
    pagination = pagination.map((source) =>
      source.serverId === item.serverId
        ? { ...source, offset: Math.max(0, source.offset - item.group.occurrences.length) }
        : source
    );
    try {
      await store.deleteOccurrences(
        item.group.occurrences.map((occurrence) => occurrence.id),
        item.group.occurrences.filter((occurrence) => occurrence.unread).length
      );
    } catch (error) {
      if (removed && !groups.some((candidate) => rowKey(candidate) === rowKey(removed))) {
        groups = [...groups, removed].sort(compareGroups);
        pagination = pagination.map((source) =>
          source.serverId === item.serverId
            ? { ...source, offset: source.offset + item.group.occurrences.length }
            : source
        );
      }
      console.error('Failed to update notification:', error);
      toast.error(m('common.error.network'));
    } finally {
      setMutationPending(key, false);
    }
  }

  async function dismissAll() {
    if (dismissingAll || hasPendingMutation || groups.length === 0) return;
    dismissingAll = true;
    const originalGroups = groups;
    const originalPagination = pagination;
    groups = [];
    pagination = pagination.map((source) => ({ ...source, offset: 0, hasMore: false }));

    const serverIds = serverRegistry.servers.flatMap((instance) => {
      const store = serverRegistry.getStore(instance.id);
      return store.isAuthenticated ? [instance.id] : [];
    });
    const results = await Promise.allSettled(
      serverIds.map((serverId) =>
        serverRegistry.getStore(serverId).notifications.deleteAllOccurrences()
      )
    );
    const failedServerIds = new Set(
      results.flatMap((result, index) => (result.status === 'rejected' ? [serverIds[index]!] : []))
    );
    if (failedServerIds.size > 0) {
      groups = [
        ...groups,
        ...originalGroups.filter(
          (item) =>
            failedServerIds.has(item.serverId) &&
            !groups.some((current) => rowKey(current) === rowKey(item))
        )
      ].sort(compareGroups);
      pagination = pagination.map((source) => {
        if (!failedServerIds.has(source.serverId)) return source;
        return (
          originalPagination.find((candidate) => candidate.serverId === source.serverId) ?? source
        );
      });
      pageError = true;
      toast.error(m('common.error.network'));
    }
    dismissingAll = false;
  }
</script>

<div class="flex h-full w-full flex-col">
  <PaneHeader
    title={m('chat.notifications.title')}
    subtitle={m('chat.notifications.subtitle')}
    showMobileNav
  >
    {#snippet actions()}
      {#if groups.length > 0 || dismissingAll}
        <Button
          variant="danger-secondary"
          size="sm"
          disabled={dismissingAll || hasPendingMutation}
          label={m('chat.notifications.clear_all')}
          onclick={dismissAll}
        >
          <span class="iconify icon-[uil--trash-alt] text-base" aria-hidden="true"></span>
          <span>{m('chat.notifications.clear_all')}</span>
        </Button>
      {/if}
    {/snippet}
  </PaneHeader>

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if loading && groups.length === 0}
      <div class="p-6 text-muted">{m('common.loading')}</div>
    {:else if pageError && groups.length === 0}
      <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.network')}>
        <Button variant="secondary" label={m('common.retry')} onclick={loadNotifications}
          >{m('common.retry')}</Button
        >
      </EmptyState>
    {:else if visibleGroups.length === 0}
      <EmptyState icon="icon-[uil--bell-slash]" title={m('chat.notifications.empty_title')}>
        {m('chat.notifications.empty_body')}
      </EmptyState>
    {:else}
      <div class="selectable-list pb-3">
        {#each dateSections as section (section.key)}
          <section aria-labelledby={`notification-date-${section.key}`}>
            <h2
              id={`notification-date-${section.key}`}
              class="sticky top-0 z-10 border-y border-border bg-background/95 px-4 py-2 text-xs font-semibold tracking-wide text-muted uppercase backdrop-blur"
              data-testid="notification-date-heading"
            >
              {section.label}
            </h2>
            {#each section.items as item (rowKey(item))}
              {@const occurrence = item.group.openTarget}
              {@const actor = occurrence?.actor ?? null}
              {@const actors = notificationActors(item.group)}
              {@const mutationPending = dismissingAll || pendingMutationKeys.has(mutationKey(item))}
              <div
                class={[
                  'group flex w-full cursor-pointer items-center gap-3 selectable-list-item px-3 py-2.5 transition-colors',
                  item.group.unread && 'bg-attention/5'
                ]}
                data-testid="notification-group"
                data-notification-state={item.group.unread ? 'unread' : 'read'}
              >
                <button
                  type="button"
                  class="flex min-w-0 flex-1 cursor-pointer items-center gap-3 rounded-md text-start focus-visible:outline-2 focus-visible:outline-action disabled:cursor-wait"
                  disabled={mutationPending}
                  onclick={() => openGroup(item)}
                >
                  {#if actors.length > 1}
                    <span
                      class="flex shrink-0 -space-x-2 rtl:space-x-reverse"
                      data-testid="notification-actor-stack"
                    >
                      {#each actors as groupedActor (groupedActor.id)}
                        <UserAvatar user={groupedActor} size="md" class="ring-2 ring-background" />
                      {/each}
                    </span>
                  {:else if actor}<UserAvatar user={actor} size="md" />{/if}
                  {#if item.group.unread}
                    <span
                      class="size-2 shrink-0 rounded-full bg-attention"
                      aria-label={m('chat.notifications.unread')}
                    ></span>
                  {/if}
                  <span class="min-w-0 flex-1">
                    <bdi class="block truncate font-medium" dir="auto">
                      {occurrenceSummary(item.group)}
                    </bdi>
                    {#if item.group.threadRootMessageExcerpt}
                      <span
                        class="mt-0.5 flex min-w-0 items-center gap-1.5 text-sm text-muted"
                        data-testid="notification-thread-root-excerpt"
                      >
                        <span
                          class="iconify icon-[uil--comment-alt-message] shrink-0 text-attention"
                          aria-hidden="true"
                        ></span>
                        <bdi class="truncate" dir="auto">
                          {item.group.threadRootMessageExcerpt}
                        </bdi>
                      </span>
                    {/if}
                    <span class="block truncate text-sm text-muted">
                      {#if showServerHostname}{item.serverHostname}<span
                          class="mx-1.5"
                          aria-hidden="true">·</span
                        >{/if}
                      {#if occurrence?.room?.name}
                        <bdi dir="auto">#{occurrence.room.name}</bdi><span
                          class="mx-1.5"
                          aria-hidden="true">·</span
                        >{/if}{formatTime(item.group.latestAt, item.timeFormatSettings)}
                    </span>
                  </span>
                </button>
                <div class="flex shrink-0 items-center gap-2">
                  <Button
                    variant="danger-secondary"
                    size="sm"
                    disabled={mutationPending}
                    label={m('common.delete')}
                    title={m('common.delete')}
                    onclick={() => dismiss(item)}
                  >
                    <span class="iconify icon-[uil--trash-alt] text-base" aria-hidden="true"></span>
                  </Button>
                </div>
              </div>
            {/each}
          </section>
        {/each}
        {#if pageError}
          <div class="flex min-h-14 items-center justify-center gap-3 p-4 text-muted" role="alert">
            <span>{m('common.error.network')}</span>
            <Button variant="secondary" size="sm" label={m('common.retry')} onclick={loadMore}
              >{m('common.retry')}</Button
            >
          </div>
        {:else if hasMore}
          <div class="flex min-h-14 justify-center p-4 text-muted" {@attach loadMoreWhenVisible}>
            {#if loadingMore}{m('common.loading')}{/if}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
