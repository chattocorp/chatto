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
    NotificationAttentionLevel,
    NotificationSignalKind,
    type NotificationActor,
    type NotificationGroupItem,
    type NotificationOccurrenceItem
  } from '$lib/api-client/notifications';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import DaySeparator from '$lib/components/DaySeparator.svelte';
  import {
    fileDateGroup,
    formatDate,
    formatMonthYear,
    timeFormatSettingsFor,
    type TimeFormatSettings
  } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { getEmojiByName } from '$lib/emoji';

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
  const hydrationAttempts = new SvelteSet<string>();
  const optimisticallyDismissedOccurrenceIds = new SvelteSet<string>();
  const groups = $derived.by(notificationGroupsFromProjection);
  const pagination = $derived.by(notificationPaginationFromProjection);
  const loading = $derived(!notificationProjectionHasLoaded());
  let loadingMore = $state(false);
  let loadMoreError = $state(false);
  let dismissingAll = $state(false);
  const pendingMutationKeys = new SvelteSet<string>();
  const hasPendingMutation = $derived(pendingMutationKeys.size > 0);
  const hasMore = $derived(pagination.some((source) => source.hasMore));
  const pageError = $derived(
    loadMoreError ||
      serverRegistry.servers.some((instance) => {
        const stores = serverRegistry.getStore(instance.id);
        return stores.isAuthenticated && stores.notifications.error !== null;
      })
  );
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

  // Realtime normally hydrates this retained store before the route is opened.
  // Fetch only genuinely missing projections as a transport fallback.
  $effect(() => {
    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (
        !stores.isAuthenticated ||
        stores.notifications.hasLoaded ||
        hydrationAttempts.has(instance.id)
      ) {
        continue;
      }
      hydrationAttempts.add(instance.id);
      void stores.notifications.fetch();
    }
  });

  // A privacy boundary can remove every renderable row from a raw page. Keep
  // advancing until content is visible or the authoritative page is exhausted.
  $effect(() => {
    if (!loading && groups.length === 0 && hasMore && !loadingMore && !pageError) {
      void loadMore();
    }
  });

  async function retryNotifications() {
    loadMoreError = false;
    const requests = serverRegistry.servers.flatMap((instance) => {
      const stores = serverRegistry.getStore(instance.id);
      if (
        !stores.isAuthenticated ||
        (stores.notifications.hasLoaded && stores.notifications.error === null)
      ) {
        return [];
      }
      hydrationAttempts.add(instance.id);
      return [stores.notifications.fetch()];
    });
    await Promise.allSettled(requests);
  }

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;
    loadingMore = true;
    loadMoreError = false;
    const pending = pagination.filter((source) => source.hasMore);
    const results = await Promise.allSettled(
      pending.map((source) =>
        serverRegistry.getStore(source.serverId).notifications.fetchPage(source.offset)
      )
    );
    loadMoreError = results.some((result) => result.status === 'rejected');
    loadingMore = false;
    if (groups.length === 0 && hasMore && !pageError) void loadMore();
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

  function visibleOccurrencesForServer(
    serverId: string,
    occurrences: NotificationOccurrenceItem[]
  ): NotificationOccurrenceItem[] {
    const notifications = serverRegistry.getStore(serverId).notifications;
    return occurrences
      .filter(
        (occurrence) => !occurrence.room || !notifications.revokedRoomIds.has(occurrence.room.id)
      )
      .map((occurrence) =>
        occurrence.actor && notifications.scrubbedUserIds.has(occurrence.actor.id)
          ? { ...occurrence, actor: null }
          : occurrence
      );
  }

  function notificationGroupsFromProjection(): ServerGroup[] {
    return serverRegistry.servers
      .flatMap((instance) => {
        const stores = serverRegistry.getStore(instance.id);
        if (!stores.isAuthenticated) return [];
        let hostname: string;
        try {
          hostname = new URL(instance.url).hostname;
        } catch {
          hostname = instance.url;
        }
        return groupNotificationOccurrences(
          visibleOccurrencesForServer(instance.id, stores.notifications.occurrences).filter(
            (occurrence) => !optimisticallyDismissedOccurrenceIds.has(occurrence.id)
          )
        ).map((group): ServerGroup => ({
          serverId: instance.id,
          serverHostname: hostname,
          timeFormatSettings: timeFormatSettingsFor(stores.currentUser.user?.settings),
          group
        }));
      })
      .sort(compareGroups);
  }

  function notificationPaginationFromProjection(): PaginationSource[] {
    return serverRegistry.servers.flatMap((instance) => {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) return [];
      return [
        {
          serverId: instance.id,
          offset: stores.notifications.consumedCount,
          hasMore: stores.notifications.hasMore
        }
      ];
    });
  }

  function notificationProjectionHasLoaded(): boolean {
    return serverRegistry.servers.every((instance) => {
      const stores = serverRegistry.getStore(instance.id);
      return !stores.isAuthenticated || stores.notifications.hasLoaded;
    });
  }

  function compareGroups(a: ServerGroup, b: ServerGroup): number {
    const byTime = b.group.latestAt.localeCompare(a.group.latestAt);
    if (byTime !== 0) return byTime;
    return mutationKey(a).localeCompare(mutationKey(b));
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
    const signalKind = occurrence.signalKind;
    const actor = occurrence.actor?.displayName;
    if (signalKind === NotificationSignalKind.REACTION) {
      const reactionOccurrence = group.occurrences.find((item) => item.actor) ?? occurrence;
      const reactionActor = reactionOccurrence.actor?.displayName ?? m('common.deleted_user');
      const emojis = [
        ...new Set(
          group.occurrences
            .map((item) => item.reactionEmoji)
            .filter((emoji): emoji is string => Boolean(emoji))
            .map((emoji) => getEmojiByName(emoji) ?? emoji)
        )
      ];
      const emoji = emojis.join(' ');
      const channel = reactionOccurrence.room?.name ?? occurrence.room?.name;
      if (emoji && channel) {
        const values = { actor: reactionActor, emoji, channel: `#${channel}` };
        const participants = new Set(
          group.occurrences.map((item) => item.actor?.id ?? 'deleted-user')
        );
        return participants.size > 1
          ? m('chat.notifications.summary.reaction_group', values)
          : m('chat.notifications.summary.reaction', values);
      }
      return m('chat.notifications.summary.activity', { actor: reactionActor });
    }
    if (!actor) return m('chat.notifications.activity');
    if (signalKind === NotificationSignalKind.DIRECT_MESSAGE) {
      return m('chat.notifications.summary.direct_message', { actor });
    }
    if (signalKind === NotificationSignalKind.ROOM_MESSAGE) {
      return m('chat.notifications.summary.new_message', { actor });
    }
    if (signalKind === NotificationSignalKind.REPLY) {
      return m('chat.notifications.summary.reply', { actor });
    }
    if (
      signalKind === NotificationSignalKind.DIRECT_MENTION ||
      signalKind === NotificationSignalKind.ROLE_MENTION ||
      signalKind === NotificationSignalKind.HERE ||
      signalKind === NotificationSignalKind.ALL
    ) {
      return m('chat.notifications.summary.mention', { actor });
    }
    if (signalKind === NotificationSignalKind.FOLLOWED_THREAD) {
      return m('chat.notifications.summary.followed_thread', { actor });
    }
    if (signalKind === NotificationSignalKind.FOLLOWED_ROOM) {
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
    if (!occurrence || occurrence.targetSupported === false) return;
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
    for (const occurrence of item.group.occurrences) {
      optimisticallyDismissedOccurrenceIds.add(occurrence.id);
    }
    try {
      await store.deleteOccurrences(
        item.group.occurrences.map((occurrence) => occurrence.id),
        {
          unread: item.group.occurrences.filter((occurrence) => occurrence.unread).length,
          importantUnread: item.group.occurrences.filter(
            (occurrence) =>
              occurrence.unread &&
              occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT
          ).length,
          roomId: item.group.openTarget?.room?.id ?? null
        }
      );
    } catch (error) {
      console.error('Failed to update notification:', error);
      toast.error(m('common.error.network'));
    } finally {
      setMutationPending(key, false);
    }
  }

  async function dismissAll() {
    if (dismissingAll || hasPendingMutation || groups.length === 0) return;
    dismissingAll = true;
    for (const item of groups) {
      for (const occurrence of item.group.occurrences) {
        optimisticallyDismissedOccurrenceIds.add(occurrence.id);
      }
    }

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
    {#if pageError && groups.length === 0}
      <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.network')}>
        <Button variant="secondary" label={m('common.retry')} onclick={retryNotifications}
          >{m('common.retry')}</Button
        >
      </EmptyState>
    {:else if visibleGroups.length > 0}
      <div class="selectable-list pb-3" aria-busy={loadingMore}>
        {#each dateSections as section (section.key)}
          <section aria-labelledby={`notification-date-${section.key}`}>
            <DaySeparator
              id={`notification-date-${section.key}`}
              label={section.label}
              testId="notification-date-heading"
            />
            {#each section.items as item (rowKey(item))}
              {@const occurrence = item.group.openTarget}
              {@const targetSupported = occurrence?.targetSupported !== false}
              {@const isReaction = occurrence?.signalKind === NotificationSignalKind.REACTION}
              {@const actor = occurrence?.actor ?? null}
              {@const actors = notificationActors(item.group)}
              {@const mutationPending = dismissingAll || pendingMutationKeys.has(mutationKey(item))}
              <div
                class={[
                  'group flex w-full items-center gap-3 selectable-list-item px-3 py-2.5 transition-colors',
                  targetSupported ? 'cursor-pointer' : 'cursor-default',
                  item.group.unread &&
                    item.group.attentionLevel === NotificationAttentionLevel.IMPORTANT &&
                    'bg-attention/5'
                ]}
                data-testid="notification-group"
                data-notification-state={item.group.unread ? 'unread' : 'read'}
                data-notification-attention={item.group.unread
                  ? item.group.attentionLevel === NotificationAttentionLevel.IMPORTANT
                    ? 'important'
                    : 'ambient'
                  : 'none'}
              >
                <button
                  type="button"
                  class={[
                    'flex min-w-0 flex-1 items-center gap-3 rounded-md text-start focus-visible:outline-2 focus-visible:outline-action',
                    targetSupported ? 'cursor-pointer' : 'cursor-default',
                    mutationPending && 'cursor-wait',
                    !item.group.unread && 'opacity-60'
                  ]}
                  disabled={mutationPending || !targetSupported}
                  onclick={() => openGroup(item)}
                >
                  <span class="flex shrink-0">
                    {#if actors.length > 1}
                      <span
                        class="flex shrink-0 -space-x-2 rtl:space-x-reverse"
                        data-testid="notification-actor-stack"
                      >
                        {#each actors as groupedActor (groupedActor.id)}
                          <UserAvatar
                            user={groupedActor}
                            size="md"
                            class="ring-2 ring-background"
                          />
                        {/each}
                      </span>
                    {:else if actor}<UserAvatar user={actor} size="md" />{/if}
                  </span>
                  {#if item.group.unread}
                    <span class="sr-only">{m('chat.notifications.unread')}</span>
                  {/if}
                  <span class="min-w-0 flex-1" data-testid="notification-content">
                    <bdi class="block truncate font-medium" dir="auto">
                      {occurrenceSummary(item.group)}
                    </bdi>
                    <span class="block truncate text-sm text-muted">
                      {#if showServerHostname}{item.serverHostname}<span
                          class="mx-1.5"
                          aria-hidden="true">·</span
                        >{/if}
                      {#if occurrence?.room?.name && !isReaction}
                        <bdi dir="auto">#{occurrence.room.name}</bdi><span
                          class="mx-1.5"
                          aria-hidden="true">·</span
                        >
                      {/if}{formatTime(item.group.latestAt, item.timeFormatSettings)}
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
          <div class="min-h-14" {@attach loadMoreWhenVisible}></div>
        {/if}
      </div>
    {:else if !loading}
      <EmptyState icon="icon-[uil--bell-slash]" title={m('chat.notifications.empty_title')}>
        {m('chat.notifications.empty_body')}
      </EmptyState>
    {/if}
  </div>
</div>
