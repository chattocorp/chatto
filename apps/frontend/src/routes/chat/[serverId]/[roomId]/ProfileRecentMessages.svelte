<!--
@component

The newest messages by one user that the viewer can read, for the Profile View.
It uses message search, so it shows only while search is available on the
server. Each message opens its place in its room or thread. Rows are
link-role elements, like search results, because a body can contain links. The section
renders nothing when the user has no visible messages or the read fails.
-->
<script lang="ts">
  import { createMessageSearchAPI, MessageSearchOrder } from '@chatto/client/api/messageSearch';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import { messagesFromQuery } from '$lib/components/users/searchUserMessages';
  import { goto } from '$app/navigation';
  import type { MessageSearchResult } from '@chatto/client/api/messageSearch';
  import { buildMessageLinkPath } from '$lib/chatLinks';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { LoadingDots } from '$lib/ui';
  import { formatRelativeTime, formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import ClampedMessagePreview from './ClampedMessagePreview.svelte';

  /** How many recent messages the Profile View shows. */
  const RECENT_MESSAGE_COUNT = 3;

  let { userId, login }: { userId: string; login: string } = $props();

  const serverScope = useServerScope();
  const searchAvailable = $derived(
    serverUi(serverScope.store).messageSearch.availability.available
  );
  const timeSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const locale = $derived(getLocale());

  function open(result: MessageSearchResult): void {
    void goto(
      buildMessageLinkPath(serverScope.serverId, result.roomId, result.id, result.threadRootEventId)
    );
  }

  function openFromKeyboard(event: KeyboardEvent, result: MessageSearchResult): void {
    if (event.target !== event.currentTarget || event.key !== 'Enter') return;
    event.preventDefault();
    open(result);
  }

  const recent = createQuery(() => {
    const connection = serverScope.connection;
    const id = userId;
    const query = messagesFromQuery(login);
    return {
      queryKey: [...serverSessionQueryRoot(serverScope), 'user', id, 'recent-messages'],
      queryFn: async () =>
        (
          await connection.getAPI(createMessageSearchAPI).searchMessages({
            query,
            authorId: id,
            order: MessageSearchOrder.NEWEST,
            pageSize: RECENT_MESSAGE_COUNT
          })
        ).results,
      enabled: searchAvailable && !!id && !!login,
      staleTime: 30_000,
      retry: false
    };
  });
</script>

{#if searchAvailable && (recent.isPending || recent.data?.length)}
  <section class="flex flex-col gap-2" data-testid="profile-recent-messages">
    <h3 class="section-label text-muted">{m('chat.profile.recent_messages')}</h3>
    {#if recent.data?.length}
      <ul class="flex flex-col gap-1.5">
        {#each recent.data as result (result.id)}
          {@const place =
            result.roomKind === RoomKind.DM
              ? m('room.title.direct_message')
              : `#${result.roomName ?? m('search.scope.room')}`}
          {@const when = result.createdAt
            ? formatRelativeTime(result.createdAt, timeSettings, locale)
            : ''}
          <li>
            <div
              role="link"
              tabindex="0"
              class="cursor-pointer tile tile-interactive px-3 py-2 focus-visible:outline-2 focus-visible:outline-action"
              onclick={() => open(result)}
              onkeydown={(event) => openFromKeyboard(event, result)}
              aria-label={[place, when, result.body.slice(0, 200)].filter(Boolean).join(' · ')}
            >
              <div class="flex min-w-0 items-center gap-1.5 text-sm text-muted" aria-hidden="true">
                <bdi class="min-w-0 truncate">{place}</bdi>
                {#if result.createdAt}
                  <span aria-hidden="true">·</span>
                  <time
                    class="shrink-0"
                    datetime={result.createdAt}
                    title={formatDateTime(result.createdAt, timeSettings, locale)}
                  >
                    {when}
                  </time>
                {/if}
              </div>
              <div class="pointer-events-none mt-1" inert>
                <ClampedMessagePreview>
                  <MessageContent body={result.body} timestampSettings={timeSettings} />
                </ClampedMessagePreview>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <LoadingDots class="h-16 w-full" />
    {/if}
  </section>
{/if}
