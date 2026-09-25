<!--
@component

Shows the full, paged member list for one message reaction. Only the selected
emoji is queried. The responsive dialog owns dismissal and scroll containment.
-->
<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { SvelteSet } from 'svelte/reactivity';
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { Code, ConnectError } from '$lib/api-client/connect';
  import { createReactionAPI } from '$lib/api-client/reactions';
  import { createUserAPI, type UserSummary } from '$lib/api-client/users';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getEmojiByName, getEmojiDisplayName } from '$lib/emoji';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import type { ReactionSummaryView } from '$lib/render/reactions';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { Dialog, LoadingFog } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  const PAGE_SIZE = 50;

  type ReactionUserRow = { id: string; user: UserSummary | null };
  type ReactionUserPage = {
    emoji: string;
    rows: ReactionUserRow[];
    nextOffset: number;
    hasMore: boolean;
  };

  let {
    roomId,
    messageEventId,
    reactions,
    onClose
  }: {
    roomId: string;
    messageEventId: string;
    reactions: ReactionSummaryView[];
    onClose: () => void;
  } = $props();

  const serverScope = useServerScope();
  const id = $props.id();
  let selectedEmoji = $state('');
  const selectedIndex = $derived(
    Math.max(
      0,
      reactions.findIndex((reaction) => reaction.emoji === selectedEmoji)
    )
  );
  const selectedReaction = $derived(reactions[selectedIndex]);
  const activeEmoji = $derived(selectedReaction?.emoji ?? '');
  // A new summary means the visible roster may have changed. It also starts
  // pagination again so live offset changes cannot mix old and new pages.
  const selectedRevision = $derived(selectedReaction ? JSON.stringify(selectedReaction) : '');

  const usersQuery = createInfiniteQuery(
    () => {
      const connection = serverScope.connection;
      const emoji = activeEmoji;
      return {
        queryKey: [
          'server',
          serverScope.serverId,
          connection.queryScope,
          'message-reaction-users',
          roomId,
          messageEventId,
          emoji,
          selectedRevision
        ],
        enabled: !!emoji && serverScope.isCurrent(),
        initialPageParam: 0,
        gcTime: 0,
        refetchOnMount: 'always' as const,
        queryFn: async ({ pageParam, signal }): Promise<ReactionUserPage> => {
          const page = await connection
            .getAPI(createReactionAPI)
            .listReactionUsers({ roomId, messageEventId, emoji }, pageParam, PAGE_SIZE, signal);
          signal.throwIfAborted();
          const profiles = page.userIds.length
            ? await connection.getAPI(createUserAPI).batchGetUsers(page.userIds)
            : [];
          signal.throwIfAborted();
          const byId = new Map(profiles.map((user) => [user.id, user]));
          return {
            emoji,
            rows: page.userIds.map((id) => ({ id, user: byId.get(id) ?? null })),
            nextOffset: pageParam + page.userIds.length,
            hasMore: page.hasMore && page.userIds.length > 0
          };
        },
        getNextPageParam: (lastPage: ReactionUserPage) =>
          lastPage.hasMore ? lastPage.nextOffset : undefined
      };
    },
    () => queryClient
  );

  const rows = $derived.by(() => {
    const seen = new SvelteSet<string>();
    return (usersQuery.data?.pages ?? []).flatMap((page) =>
      page.emoji === activeEmoji
        ? page.rows.filter((row) => {
            if (seen.has(row.id)) return false;
            seen.add(row.id);
            return true;
          })
        : []
    );
  });
  const accessLost = $derived(
    usersQuery.error instanceof ConnectError &&
      [Code.PermissionDenied, Code.NotFound, Code.Unauthenticated].includes(usersQuery.error.code)
  );
  const loadMoreWhenVisible = useLoadMoreWhenVisible({
    getCursor: () =>
      usersQuery.hasNextPage ? (usersQuery.data?.pages.at(-1)?.nextOffset ?? null) : null,
    loadMore: async () => {
      await usersQuery.fetchNextPage();
    },
    hasError: () => usersQuery.isError
  });

  function selectEmoji(index: number): void {
    const emoji = reactions[index]?.emoji;
    if (!emoji) return;
    selectedEmoji = emoji;
    document.getElementById(`${id}-reaction-tab-${index}`)?.scrollIntoView({
      block: 'nearest',
      inline: 'nearest'
    });
  }

  function handleTabKeydown(event: KeyboardEvent): void {
    const rtl = getComputedStyle(event.currentTarget as HTMLElement).direction === 'rtl';
    let next = selectedIndex;
    if (event.key === 'ArrowRight') next += rtl ? -1 : 1;
    else if (event.key === 'ArrowLeft') next += rtl ? 1 : -1;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = reactions.length - 1;
    else return;
    event.preventDefault();
    selectEmoji((next + reactions.length) % reactions.length);
    document
      .getElementById(`${id}-reaction-tab-${(next + reactions.length) % reactions.length}`)
      ?.focus();
  }
</script>

<Dialog visible title={m('room.message.actions.reactions')} size="sm" onclose={onClose}>
  <div class="flex min-h-0 flex-col gap-3">
    <div
      role="tablist"
      tabindex="-1"
      aria-label={m('room.message.actions.reactions')}
      class="flex max-h-24 flex-wrap gap-1.5 overflow-y-auto overscroll-contain border-b border-border pb-3"
      onkeydown={handleTabKeydown}
    >
      {#each reactions as reaction, index (reaction.emoji)}
        <button
          id={`${id}-reaction-tab-${index}`}
          type="button"
          role="tab"
          aria-selected={index === selectedIndex}
          aria-controls={`${id}-reaction-panel`}
          aria-label={`${getEmojiDisplayName(reaction.emoji)} (${reaction.count})`}
          tabindex={index === selectedIndex ? 0 : -1}
          class={[
            'meta-badge min-h-10 shrink-0 gap-1.5 px-3 text-sm',
            index === selectedIndex ? 'border-action/50 text-text' : 'border-transparent text-muted'
          ]}
          onclick={() => selectEmoji(index)}
        >
          <span aria-hidden="true">{getEmojiByName(reaction.emoji) ?? reaction.emoji}</span>
          <span aria-hidden="true">{reaction.count}</span>
        </button>
      {/each}
    </div>

    <div
      id={`${id}-reaction-panel`}
      role="tabpanel"
      aria-labelledby={`${id}-reaction-tab-${selectedIndex}`}
      class="min-h-24"
      data-testid="reaction-details-panel"
    >
      {#if usersQuery.isPending}
        <LoadingFog class="h-24 w-full" />
      {:else if accessLost || (usersQuery.isError && !usersQuery.isFetchNextPageError)}
        <div class="flex flex-col items-center gap-3 py-6 text-center" role="alert">
          <p>{m(accessLost ? 'common.error.generic' : 'common.error.network')}</p>
          {#if !accessLost}
            <Button variant="secondary" onclick={() => void usersQuery.refetch()}>
              {m('common.retry')}
            </Button>
          {/if}
        </div>
      {:else}
        <ul class="flex flex-col gap-1" aria-live="polite">
          {#each rows as row (row.id)}
            {@const deleted =
              serverScope.store.projection.users.isDeleted(row.id) || !!row.user?.deleted}
            {@const user = deleted ? null : row.user}
            <li
              class="flex min-h-12 items-center gap-3 rounded px-2 py-1"
              data-testid="reaction-details-user"
            >
              {#if user}
                <UserAvatar
                  user={{ ...user, presenceStatus: PresenceStatus.OFFLINE }}
                  serverId={serverScope.serverId}
                  size="sm"
                  showPresence={false}
                />
              {:else}
                <span
                  class="iconify icon-[uil--user] h-8 w-8 shrink-0 text-xl text-muted"
                  aria-hidden="true"
                ></span>
              {/if}
              <span class="min-w-0">
                <AccountName
                  name={user
                    ? getLiveDisplayName(user.id, user.displayName || user.login)
                    : deleted
                      ? m('common.deleted_user')
                      : m('common.unknown_user')}
                  identity={user}
                />
              </span>
            </li>
          {/each}
        </ul>
        {#if usersQuery.hasNextPage}
          <div class="flex min-h-12 items-center justify-center" {@attach loadMoreWhenVisible}>
            {#if usersQuery.isFetchNextPageError}
              <Button variant="secondary" onclick={() => void usersQuery.fetchNextPage()}>
                {m('common.retry')}
              </Button>
            {:else}
              <LoadingFog class="h-10 w-full" />
            {/if}
          </div>
        {/if}
      {/if}
    </div>
  </div>
</Dialog>
