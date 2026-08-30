<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { ActivityListRow, EmptyState, PaneHeader } from '$lib/ui';
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
  import UserAvatarStack from '$lib/components/UserAvatarStack.svelte';
  import DaySeparator from '$lib/components/DaySeparator.svelte';
  import {
    formatRelativeTime,
    groupByActivityDate,
    timeFormatSettingsFor,
    type TimeFormatSettings
  } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { getEmojiByName } from '$lib/emoji';
  import {
    enablePushOnAllServers,
    getPermission,
    getPushCapability,
    getPushRegistrationTargets
  } from '$lib/notifications/pushNotifications';

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
  let dismissingRead = $state(false);
  let pushPermission = $state<NotificationPermission | null>(getPermission());
  let enablingPush = $state(false);
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
  const dateSections = $derived.by(() =>
    groupByActivityDate(
      visibleGroups,
      (item) => item.group.latestAt,
      (item) => item.timeFormatSettings,
      new Date(),
      activeLocale
    )
  );
  const readOccurrenceBatches = $derived.by(readOccurrencesByServer);
  const showEnablePush = $derived(
    getPushCapability() === 'supported' &&
      pushPermission === 'default' &&
      getPushRegistrationTargets().length > 0
  );

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

  function occurrenceKey(serverId: string, occurrenceId: string): string {
    return `${serverId}:${occurrenceId}`;
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
            (occurrence) =>
              !optimisticallyDismissedOccurrenceKeys.has(occurrenceKey(instance.id, occurrence.id))
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
    if (dismissingRead || hasPendingMutation || readOccurrenceBatches.length === 0) return;
    dismissingRead = true;
    const batches = readOccurrenceBatches.map((batch) => ({
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

  async function enablePushNotifications() {
    if (enablingPush) return;
    enablingPush = true;
    try {
      const result = await enablePushOnAllServers();
      pushPermission = result.permission;
      if (result.permission === 'denied') {
        toast.error(m('settings.notifications.push_prompt.blocked'));
      } else if (
        result.permission === 'granted' &&
        result.registrations.length > 0 &&
        result.registrations.every((registration) => registration.registered)
      ) {
        toast.success(m('settings.notifications.push_prompt.enabled'));
      } else if (result.permission === 'granted') {
        toast.error(m('settings.notifications.push_prompt.enable_failed'));
      }
    } catch {
      toast.error(m('settings.notifications.push_prompt.enable_failed'));
    } finally {
      enablingPush = false;
    }
  }
</script>

<div class="flex h-full w-full flex-col">
  <PaneHeader
    title={m('chat.notifications.title')}
    subtitle={m('chat.notifications.subtitle')}
    showMobileNav
  >
    {#snippet actions()}
      {#if showEnablePush}
        <Button
          size="sm"
          disabled={enablingPush}
          loading={enablingPush}
          loadingText={m('settings.notifications.push_prompt.enabling')}
          label={m('settings.notifications.push_prompt.title')}
          onclick={enablePushNotifications}
        >
          <span class="iconify icon-[uil--bell] text-base" aria-hidden="true"></span>
          <span>{m('settings.notifications.push_prompt.title')}</span>
        </Button>
      {/if}
      {#if readOccurrenceBatches.length > 0 || dismissingRead}
        <Button
          variant="danger-secondary"
          size="sm"
          disabled={dismissingRead || hasPendingMutation}
          label={m('chat.notifications.clear_read')}
          onclick={dismissRead}
        >
          <span class="iconify icon-[uil--trash-alt] text-base" aria-hidden="true"></span>
          <span>{m('chat.notifications.clear_read')}</span>
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
              {@const actors = notificationActors(item.group)}
              {@const mutationPending =
                dismissingRead || pendingMutationKeys.has(mutationKey(item))}
              <ActivityListRow
                interactive={targetSupported}
                pending={mutationPending}
                disabled={mutationPending || !targetSupported}
                dimmed={!item.group.unread}
                important={item.group.unread &&
                  item.group.attentionLevel === NotificationAttentionLevel.IMPORTANT}
                onclick={() => openGroup(item)}
                rowAttributes={{
                  'data-testid': 'notification-group',
                  'data-notification-state': item.group.unread ? 'unread' : 'read',
                  'data-notification-attention': item.group.unread
                    ? item.group.attentionLevel === NotificationAttentionLevel.IMPORTANT
                      ? 'important'
                      : 'ambient'
                    : 'none'
                }}
              >
                {#snippet leading()}
                  <UserAvatarStack users={actors} testId="notification-actor-stack" />
                {/snippet}
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
                    {/if}{formatRelativeTime(
                      item.group.latestAt,
                      item.timeFormatSettings,
                      activeLocale
                    )}
                  </span>
                </span>
                {#snippet actions()}
                  <button
                    type="button"
                    class="icon-action hover:text-danger focus-visible:text-danger"
                    disabled={mutationPending}
                    aria-label={m('common.delete')}
                    title={m('common.delete')}
                    onclick={() => dismiss(item)}
                  >
                    <span class="iconify icon-[uil--trash-alt] text-base" aria-hidden="true"></span>
                  </button>
                {/snippet}
              </ActivityListRow>
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
