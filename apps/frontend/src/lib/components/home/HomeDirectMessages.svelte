<!--
@component

The direct messages of every signed-in server on the Home page. Rows that need
attention come first; see `homeDirectMessages`.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import { EmptyState, LoadingFog, NotificationBadge, Panel, UnreadDot } from '$lib/ui';
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import UserAvatarStack from '$lib/components/UserAvatarStack.svelte';
  import { buildDirectMessagePresentation } from '@chatto/client/timeline/users';
  import { directMessageLabels } from '$lib/render/directMessageLabels';
  import type { HomeDirectMessage } from '$lib/home/homeOverview';

  let {
    directMessages,
    loading,
    showServerName
  }: {
    directMessages: HomeDirectMessage[];
    /** Show the loading state when no direct message is known yet. */
    loading: boolean;
    /** Name the server of each row; useful only with more than one server. */
    showServerName: boolean;
  } = $props();
</script>

<Panel title={m('room_list.direct_messages')} noPadding>
  {#if directMessages.length > 0}
    <ul class="selectable-list" data-testid="home-direct-messages">
      {#each directMessages as entry (`${entry.serverId}:${entry.room.id}`)}
        {@const presentation = buildDirectMessagePresentation(
          entry.room.members,
          entry.viewerId,
          directMessageLabels()
        )}
        {@const importantCount = entry.room.viewerImportantNotificationCount}
        {@const notificationCount = entry.room.viewerNotificationCount}
        <li>
          <a
            href={resolve('/chat/[serverId]/[roomId]', {
              serverId: serverIdToSegment(entry.serverId),
              roomId: entry.room.id
            })}
            class={[
              'flex cursor-pointer items-center gap-3 selectable-list-item px-3 py-2',
              !entry.unread && notificationCount === 0 && 'text-muted'
            ]}
            data-testid="home-direct-message"
          >
            <UserAvatarStack
              users={presentation.visibleParticipants.slice(0, 3)}
              useLiveProfile={false}
            />
            <span class="min-w-0 flex-1">
              <span
                class={[
                  'block truncate',
                  (entry.unread || notificationCount > 0) && 'font-medium text-text-top'
                ]}
              >
                <DirectMessageName
                  participants={entry.room.members}
                  currentUserId={entry.viewerId}
                />
              </span>
              {#if showServerName}
                <span class="block truncate text-sm text-muted">{entry.serverName}</span>
              {/if}
            </span>
            {#if entry.hasActiveCall}
              <span
                class="iconify icon-[uil--phone] shrink-0 text-action"
                role="img"
                aria-label={m('room_list.active_call')}
              ></span>
            {/if}
            {#if notificationCount > 0}
              {@const otherCount = Math.max(0, notificationCount - importantCount)}
              <!-- Match the sidebar: the Important count stays at the row edge. -->
              <span class="flex shrink-0 items-center gap-1">
                {#if otherCount > 0}
                  <NotificationBadge count={otherCount} color="ambient" />
                  <span class="sr-only">
                    {m('room_list.other_notifications', { count: otherCount })}
                  </span>
                {/if}
                {#if importantCount > 0}
                  <NotificationBadge count={importantCount} color="warning" />
                  <span class="sr-only">
                    {m('room_list.new_direct_messages', { count: importantCount })}
                  </span>
                {/if}
              </span>
            {:else if entry.unread}
              <UnreadDot color="neutral" />
              <span class="sr-only">{m('room_list.unread_messages')}</span>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {:else if loading}
    <LoadingFog class="m-3 min-h-24" />
  {:else}
    <div class="flex flex-col p-6">
      <EmptyState
        icon="icon-[uil--comment-alt-message]"
        title={m('chat.home.direct_messages_empty')}
      ></EmptyState>
    </div>
  {/if}
</Panel>
