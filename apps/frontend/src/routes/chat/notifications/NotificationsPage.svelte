<script lang="ts">
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { accountNameToken } from '@chatto/client/timeline/accountName';
  import { serverUi } from '$lib/state/server/serverUi';
  import { goto } from '$app/navigation';
  import { base, resolve } from '$app/paths';
  import { page } from '$app/state';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import {
    EmptyState,
    LoadingFog,
    PageTitle,
    SegmentedControl,
    PaneHeader,
    ScrollFader
  } from '$lib/ui';
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
  } from '@chatto/client/api/notifications';
  import {
    prepareUiForNotificationTarget,
    navigateNotification
  } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { serverRegistry } from '$lib/client';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverDisplayName } from '@chatto/client/server/state';
  import DaySeparator from '$lib/components/DaySeparator.svelte';
  import NotificationRow from './NotificationRow.svelte';
  import type { ActivityServer } from '$lib/components/ActivityAvatar.svelte';
  import {
    formatMessageTime,
    groupByActivityDay,
    timeFormatSettingsFor,
    type TimeFormatSettings
  } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { getEmojiByName, getReactionDisplay } from '$lib/emoji';

  let { backHref }: { backHref?: string } = $props();

  type ServerGroup = {
    serverId: string;
    /** Server marker data for multi-server mode. */
    server: ActivityServer;
    timeFormatSettings: TimeFormatSettings;
    group: NotificationGroupItem;
  };

  type PaginationSource = {
    serverId: string;
    offset: number;
    hasMore: boolean;
  };

  type ReadOccurrenceBatch = {
    serverId: string;
    occurrenceIds: string[];
  };

  const activeLocale = $derived(getLocale());
  const appUi = getAppUiState();
  const hydrationAttempts = new SvelteSet<string>();
  const optimisticallyDismissedOccurrenceKeys = new SvelteSet<string>();
  const groups = $derived.by(notificationGroupsFromProjection);
  const pagination = $derived.by(notificationPaginationFromProjection);
  const loading = $derived(!notificationProjectionHasLoaded());
  let loadingMore = $state(false);
  let loadMoreError = $state(false);
  const filter = $derived(page.url.searchParams.get('status') === 'unread' ? 'unread' : 'all');
  const kind = $derived.by(() => {
    const value = page.url.searchParams.get('type');
    return value === 'reactions' || value === 'replies' || value === 'mentions' ? value : 'all';
  });

  /** Replace the inbox entry so Back from a notification restores its filters. */
  function setFilter(parameter: 'status' | 'type', value: string) {
    const url = new URL(page.url);
    if (value === 'all') url.searchParams.delete(parameter);
    else url.searchParams.set(parameter, value);
    return goto(resolve((url.pathname.slice(base.length) + url.search + url.hash) as '/'), {
      replaceState: true,
      keepFocus: true,
      noScroll: true
    });
  }
  const filterOptions = $derived([
    { value: 'all' as const, label: m('chat.notifications.all_notifications') },
    { value: 'unread' as const, label: m('chat.threads.filter_unread') }
  ]);
  const kindOptions = $derived([
    { value: 'all' as const, label: m('chat.notifications.all_types') },
    { value: 'replies' as const, label: m('chat.notifications.replies') },
    { value: 'mentions' as const, label: m('chat.notifications.mentions') },
    { value: 'reactions' as const, label: m('chat.notifications.reactions') }
  ]);
  let dismissingRead = $state(false);
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
  const showServerMarker = $derived(
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
  const filteredGroups = $derived(
    visibleGroups.filter(({ group }) => {
      if (filter === 'unread' && !group.unread) return false;
      return group.occurrences.some(({ signalKind }) => {
        if (kind === 'reactions') return signalKind === NotificationSignalKind.REACTION;
        if (kind === 'replies')
          return (
            signalKind === NotificationSignalKind.REPLY ||
            signalKind === NotificationSignalKind.FOLLOWED_THREAD
          );
        if (kind === 'mentions')
          return (
            signalKind === NotificationSignalKind.DIRECT_MENTION ||
            signalKind === NotificationSignalKind.ROLE_MENTION ||
            signalKind === NotificationSignalKind.HERE ||
            signalKind === NotificationSignalKind.ALL
          );
        return true;
      });
    })
  );
  const dateSections = $derived.by(() =>
    groupByActivityDay(
      filteredGroups,
      (item) => item.group.latestAt,
      (item) => item.timeFormatSettings,
      new Date(),
      activeLocale
    )
  );
  const readOccurrenceBatches = $derived.by(readOccurrencesByServer);

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

  // Filters and privacy boundaries can hide a complete page. Continue until
  // a matching row is visible or every server has exhausted its pages.
  $effect(() => {
    if (
      !loading &&
      filteredGroups.length === 0 &&
      hasMore &&
      !loadingMore &&
      !dismissingRead &&
      !pageError
    ) {
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
    if (loading || loadingMore || dismissingRead || !hasMore) return;
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

  function occurrenceKey(serverId: string, occurrenceId: string): string {
    return `${serverId}:${occurrenceId}`;
  }

  function visibleOccurrencesForServer(
    serverId: string,
    occurrences: NotificationOccurrenceItem[]
  ): NotificationOccurrenceItem[] {
    const notifications = serverRegistry.getStore(serverId).notifications;
    return occurrences
      .filter((occurrence) => notifications.canDisplayOccurrence(occurrence))
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
        const server = {
          name: serverDisplayName(stores.serverInfo, instance.name),
          hostname,
          logoUrl: stores.serverInfo.iconUrl ?? instance.iconUrl ?? null
        };
        return groupNotificationOccurrences(
          visibleOccurrencesForServer(instance.id, stores.notifications.occurrences).filter(
            (occurrence) =>
              !optimisticallyDismissedOccurrenceKeys.has(occurrenceKey(instance.id, occurrence.id))
          )
        ).map((group): ServerGroup => ({
          serverId: instance.id,
          server,
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

  function readOccurrencesByServer(): ReadOccurrenceBatch[] {
    const occurrencesByServer = new SvelteMap<string, string[]>();
    for (const item of groups) {
      for (const occurrence of item.group.occurrences) {
        if (occurrence.unread) continue;
        const occurrenceIds = occurrencesByServer.get(item.serverId) ?? [];
        occurrenceIds.push(occurrence.id);
        occurrencesByServer.set(item.serverId, occurrenceIds);
      }
    }
    return [...occurrencesByServer].map(([serverId, occurrenceIds]) => ({
      serverId,
      occurrenceIds
    }));
  }

  function compareGroups(a: ServerGroup, b: ServerGroup): number {
    const byTime = b.group.latestAt.localeCompare(a.group.latestAt);
    if (byTime !== 0) return byTime;
    return mutationKey(a).localeCompare(mutationKey(b));
  }

  function setMutationPending(key: string, pending: boolean): void {
    if (pending) pendingMutationKeys.add(key);
    else pendingMutationKeys.delete(key);
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
    await goto(conversationPath(serverId, roomId, { threadId: occurrence.threadRootId }));
  }

  function occurrenceSummary(group: NotificationGroupItem): string {
    const occurrence = group.openTarget;
    if (!occurrence) return m('chat.notifications.activity');
    const signalKind = occurrence.signalKind;
    const actor = occurrence.actor ? accountNameToken(0) : undefined;
    if (signalKind === NotificationSignalKind.REACTION) {
      const reactionOccurrence = group.occurrences.find((item) => item.actor) ?? occurrence;
      const reactionActor = reactionOccurrence.actor
        ? accountNameToken(0)
        : m('common.deleted_user');
      const emojis = [
        ...new Set(
          group.occurrences
            .map((item) => item.reactionEmoji)
            .filter((emoji): emoji is string => Boolean(emoji))
            .map(renderEmoji)
        )
      ];
      const emoji = emojis.join(' ');
      if (emoji) {
        const values = { actor: reactionActor, emoji };
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
    if (signalKind === NotificationSignalKind.ROOM_INVITATION) {
      return m('chat.notifications.summary.room_invitation', { actor });
    }
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

  function summaryAccounts(group: NotificationGroupItem) {
    const actor = primaryActor(group);
    return actor ? [{ name: actor.displayName, identity: actor }] : [];
  }

  /** The actor shown as the row avatar: the summary's named actor, when still visible. */
  function primaryActor(group: NotificationGroupItem): NotificationActor | null {
    const occurrence = group.openTarget;
    if (occurrence?.signalKind === NotificationSignalKind.REACTION) {
      return group.occurrences.find((item) => item.actor)?.actor ?? null;
    }
    return occurrence?.actor ?? null;
  }

  /** Room shown on the metadata line, for every cause. */
  function groupRoomName(group: NotificationGroupItem): string | null {
    return (
      group.openTarget?.room?.name ??
      group.occurrences.find((item) => item.room)?.room?.name ??
      null
    );
  }

  /** Only glyphs fit the avatar badge; unknown names use the reaction icon. */
  function badgeEmoji(group: NotificationGroupItem): string | null {
    const name = group.occurrences.find((item) => item.reactionEmoji)?.reactionEmoji;
    return name ? (getEmojiByName(name) ?? null) : null;
  }

  /** Stored reaction emoji names render as their emoji when known. */
  function renderEmoji(name: string): string {
    return getReactionDisplay(name);
  }

  async function openGroup(item: ServerGroup) {
    const key = mutationKey(item);
    if (dismissingRead || pendingMutationKeys.has(key)) return;
    const occurrence = item.group.openTarget;
    if (!occurrence || occurrence.targetSupported === false) return;
    setMutationPending(key, true);
    try {
      const stores = serverRegistry.getStore(item.serverId);
      const roomId = occurrence.room?.id ?? null;
      prepareUiForNotificationTarget(appUi, item.serverId, { roomId });
      if (roomId && occurrence.eventId) {
        serverUi(stores).pendingHighlights.set(
          roomId,
          occurrence.threadRootId,
          occurrence.eventId,
          occurrence.unread ? occurrence.id : null
        );
      }
      await navigateNotification(stores, occurrence, () =>
        navigateToDestination(item.serverId, occurrence)
      );
    } catch (error) {
      console.error('Failed to open notification:', error);
      toast.error(m('common.error.network'));
    } finally {
      setMutationPending(key, false);
    }
  }

  async function dismiss(item: ServerGroup) {
    const key = mutationKey(item);
    if (dismissingRead || pendingMutationKeys.has(key)) return;
    setMutationPending(key, true);
    const store = serverRegistry.getStore(item.serverId).notifications;
    for (const occurrence of item.group.occurrences) {
      optimisticallyDismissedOccurrenceKeys.add(occurrenceKey(item.serverId, occurrence.id));
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

  async function dismissRead() {
    if (dismissingRead || loadingMore || hasPendingMutation) return;
    dismissingRead = true;
    // Finish pagination before deleting anything: deletions shift page offsets.
    while (notificationPaginationFromProjection().some((source) => source.hasMore)) {
      const loads = await Promise.allSettled(
        notificationPaginationFromProjection()
          .filter((source) => source.hasMore)
          .map((source) => serverRegistry.getStore(source.serverId).notifications.fetchAllPages())
      );
      if (loads.some((result) => result.status === 'rejected')) {
        toast.error(m('common.error.network'));
        dismissingRead = false;
        return;
      }
      // Another server's projection can change while its peers are loading.
    }
    const batches = readOccurrencesByServer().map((batch) => ({
      serverId: batch.serverId,
      occurrenceIds: [...batch.occurrenceIds]
    }));
    for (const batch of batches) {
      for (const occurrenceId of batch.occurrenceIds) {
        optimisticallyDismissedOccurrenceKeys.add(occurrenceKey(batch.serverId, occurrenceId));
      }
    }
    const results = await Promise.allSettled(
      batches.map((batch) =>
        serverRegistry
          .getStore(batch.serverId)
          .notifications.deleteOccurrences(batch.occurrenceIds, {
            unread: 0,
            importantUnread: 0
          })
      )
    );
    if (results.some((result) => result.status === 'rejected')) {
      toast.error(m('common.error.network'));
    }
    dismissingRead = false;
  }
</script>

<PageTitle title={m('chat.notifications.title')} />

<div class="@container/pane-header pane-page">
  <PaneHeader
    title={m('chat.notifications.title')}
    {backHref}
    backLabel={m('chat.server_nav.back_to_server')}
  >
    {#snippet actions()}
      {#if readOccurrenceBatches.length > 0 || hasMore || dismissingRead}
        <button
          type="button"
          class="pane-header-label-button text-sm"
          title={m('chat.notifications.clear_read')}
          aria-label={m('chat.notifications.clear_read')}
          disabled={dismissingRead || loadingMore || hasPendingMutation}
          onclick={dismissRead}
        >
          <span class="icon-[uil--trash-alt] pane-header-icon-glyph" aria-hidden="true"></span>
          <span class="sr-only @min-[48rem]/pane-header:not-sr-only">
            {m('chat.notifications.clear_read')}
          </span>
        </button>
      {/if}
    {/snippet}
  </PaneHeader>

  <div class="flex min-h-0 max-w-pane flex-1 flex-col">
    <div class="flex shrink-0 flex-wrap items-center gap-x-6 gap-y-3 px-4 py-3">
      <div class="max-w-full overflow-x-auto">
        <SegmentedControl
          label={m('chat.notifications.filter_status')}
          options={filterOptions}
          value={filter}
          onchange={(value) => setFilter('status', value)}
        />
      </div>
      <div class="max-w-full overflow-x-auto">
        <SegmentedControl
          label={m('chat.notifications.filter_type')}
          options={kindOptions}
          value={kind}
          onchange={(value) => setFilter('type', value)}
        />
      </div>
    </div>
    <ScrollFader
      top
      bottom
      keyboardFocusable={false}
      class="min-h-0 flex-1 px-3 [&_section:first-child>h2]:mt-0"
    >
      <div class="flex min-h-full flex-col">
        {#if pageError && filteredGroups.length === 0}
          <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.network')}>
            <Button variant="secondary" label={m('common.retry')} onclick={retryNotifications}
              >{m('common.retry')}</Button
            >
          </EmptyState>
        {:else if filteredGroups.length > 0}
          <div class="selectable-list pb-3" aria-busy={loadingMore}>
            {#each dateSections as section (section.key)}
              <section
                aria-label={section.key === 'today' ? section.label : undefined}
                aria-labelledby={section.key === 'today'
                  ? undefined
                  : `notification-date-${section.key}`}
              >
                {#if section.key !== 'today'}
                  <DaySeparator
                    id={`notification-date-${section.key}`}
                    label={section.label}
                    testId="notification-date-heading"
                  />
                {/if}
                {#each section.items as item (rowKey(item))}
                  {@const occurrence = item.group.openTarget}
                  {@const targetSupported = occurrence?.targetSupported !== false}
                  {@const isReaction = occurrence?.signalKind === NotificationSignalKind.REACTION}
                  {@const actor = primaryActor(item.group)}
                  {@const emoji = isReaction ? badgeEmoji(item.group) : null}
                  {@const roomName = groupRoomName(item.group)}
                  {@const mutationPending =
                    dismissingRead || pendingMutationKeys.has(mutationKey(item))}
                  <NotificationRow
                    summary={occurrenceSummary(item.group)}
                    summaryAccounts={summaryAccounts(item.group)}
                    {actor}
                    signalKind={occurrence?.signalKind}
                    reactionEmoji={emoji}
                    {roomName}
                    time={formatMessageTime(
                      item.group.latestAt,
                      item.timeFormatSettings,
                      activeLocale
                    )}
                    unread={item.group.unread}
                    important={item.group.unread &&
                      item.group.attentionLevel === NotificationAttentionLevel.IMPORTANT}
                    interactive={targetSupported}
                    pending={mutationPending}
                    server={showServerMarker ? item.server : null}
                    onopen={() => openGroup(item)}
                    ondismiss={() => dismiss(item)}
                  />
                {/each}
              </section>
            {/each}
            {#if pageError}
              <div
                class="flex min-h-14 items-center justify-center gap-3 p-4 text-muted"
                role="alert"
              >
                <span>{m('common.error.network')}</span>
                <Button variant="secondary" size="sm" label={m('common.retry')} onclick={loadMore}
                  >{m('common.retry')}</Button
                >
              </div>
            {:else if hasMore}
              <div class="min-h-14" {@attach loadMoreWhenVisible}></div>
            {/if}
          </div>
        {:else if loading || hasMore}
          <LoadingFog class="m-3 min-h-32 flex-1" />
        {:else}
          <EmptyState
            icon="icon-[uil--bell-slash]"
            title={filter === 'unread'
              ? m('chat.notifications.no_unread')
              : m('chat.notifications.empty_title')}
          >
            {#if filter === 'all' && kind === 'all'}{m('chat.notifications.empty_body')}{/if}
          </EmptyState>
        {/if}
      </div>
    </ScrollFader>
  </div>
</div>
