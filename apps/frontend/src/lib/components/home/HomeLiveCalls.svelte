<!--
@component

The calls in progress on every signed-in server, on the Home page. Each row
opens the room of the call. The page renders this panel only when a call is in
progress.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import { Panel } from '$lib/ui';
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import UserAvatarStack from '$lib/components/UserAvatarStack.svelte';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms';
  import type { HomeLiveCall } from '$lib/home/homeOverview';

  let {
    calls,
    showServerName
  }: {
    calls: HomeLiveCall[];
    /** Name the server of each row; useful only with more than one server. */
    showServerName: boolean;
  } = $props();

  function avatarUser(participant: CallRoomParticipant): UserAvatarUserView {
    return {
      id: participant.userId,
      login: participant.login,
      displayName: participant.displayName,
      deleted: false,
      isBot: participant.isBot,
      avatarUrl: participant.avatarUrl,
      presenceStatus: PresenceStatus.OFFLINE
    };
  }
</script>

<Panel title={m('chat.home.live_calls_title')} noPadding count={calls.length}>
  <ul class="selectable-list" data-testid="home-live-calls">
    {#each calls as call (`${call.serverId}:${call.room.id}`)}
      {@const details = [
        showServerName ? call.serverName : null,
        call.participants.length > 0
          ? m('chat.home.call_participants_count', { count: call.participants.length })
          : null
      ].filter((detail) => detail !== null)}
      <li>
        <a
          href={resolve('/chat/[serverId]/[roomId]', {
            serverId: serverIdToSegment(call.serverId),
            roomId: call.room.id
          })}
          class="flex cursor-pointer items-center gap-3 selectable-list-item px-3 py-2"
          data-testid="home-live-call"
        >
          <span class="relative inline-flex shrink-0 text-action">
            <span
              class="iconify absolute inset-0 icon-[uil--phone] animate-ping text-lg opacity-45 motion-reduce:hidden"
              aria-hidden="true"
            ></span>
            <span class="iconify relative icon-[uil--phone] text-lg" aria-hidden="true"></span>
          </span>
          <span class="min-w-0 flex-1">
            <bdi class="block truncate font-medium text-text-top" dir="auto">
              {#if call.room.type === RoomKind.DM}
                <DirectMessageName participants={call.room.members} currentUserId={call.viewerId} />
              {:else}
                #{call.room.name}
              {/if}
            </bdi>
            {#if details.length > 0}
              <span class="block truncate text-sm text-muted">{details.join(' · ')}</span>
            {/if}
          </span>
          <UserAvatarStack
            users={call.participants.slice(0, 4).map(avatarUser)}
            useLiveProfile={false}
          />
        </a>
      </li>
    {/each}
  </ul>
</Panel>
