<!--
@component

The "Happening now" panel of the server Overview: a row for each active call
in a joined room, then the joined rooms with unread messages or
notifications.
Renders nothing when there is no activity. The owner supplies the call
action, so this component does not depend on app-shell state.
-->
<script lang="ts" module>
  import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms';

  /** One active call that the Overview shows. */
  export type OverviewCall = {
    roomId: string;
    roomName: string;
    participants: CallRoomParticipant[];
  };
</script>

<script lang="ts">
  import { resolve } from '$app/paths';
  import { callParticipantAvatarUser } from '$lib/state/server/activeCallRooms';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { m } from '$lib/i18n/messages';
  import type { CatchUpRoom } from '$lib/serverOverview';
  import type { NotificationAttentionLevel } from '@chatto/client/api/notifications';
  import { Panel, PulseIcon, UnreadDot } from '$lib/ui';
  import RoomAttentionBadges from './RoomAttentionBadges.svelte';
  import { Button } from '$lib/ui/form';

  let {
    serverSegment,
    calls,
    catchUp,
    onjoincall,
    onopennotification
  }: {
    serverSegment: string;
    calls: readonly OverviewCall[];
    catchUp: readonly CatchUpRoom[];
    /** Open the call of a room. */
    onjoincall: (roomId: string) => void;
    /** Open the newest notification of a room at an attention level. */
    onopennotification: (roomId: string, attentionLevel: NotificationAttentionLevel) => void;
  } = $props();

  /** Avatars shown for each call; an overflow count follows them. */
  const VISIBLE_PARTICIPANTS = 3;

  function roomHref(roomId: string) {
    return resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId });
  }
</script>

{#if calls.length > 0 || catchUp.length > 0}
  <Panel title={m('chat.overview.happening_now')} noPadding>
    {#if calls.length > 0}
      <ul class="selectable-list" data-testid="server-overview-calls">
        {#each calls as call (call.roomId)}
          {@const overflow = call.participants.length - VISIBLE_PARTICIPANTS}
          <!-- One row per call: the room, who is in it, and the join action. -->
          <li
            class="flex min-w-0 items-center gap-2 selectable-list-item pe-2"
            data-testid="server-overview-call"
          >
            <a
              class="flex min-w-0 flex-1 items-center gap-2 py-1.5 ps-3"
              href={roomHref(call.roomId)}
            >
              <span class="sidebar-icon text-action"><PulseIcon icon="icon-[uil--phone]" /></span>
              <bdi class="min-w-0 truncate font-medium">{call.roomName}</bdi>
            </a>
            <span
              class="flex shrink-0 items-center -space-x-1"
              role="img"
              aria-label={m('chat.overview.call_participants_count', {
                count: call.participants.length
              })}
            >
              {#each call.participants.slice(0, VISIBLE_PARTICIPANTS) as participant (participant.userId)}
                <span class="inline-flex rounded-full ring-2 ring-surface" aria-hidden="true">
                  <UserAvatar
                    user={callParticipantAvatarUser(participant)}
                    size="xs"
                    useLiveProfile={false}
                  />
                </span>
              {/each}
              {#if overflow > 0}
                <span
                  aria-hidden="true"
                  class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-surface-emphasized px-1 text-xs text-muted ring-2 ring-surface"
                  >+{overflow}</span
                >
              {/if}
            </span>
            <!-- The short visible label is part of the accessible name, which names the call. -->
            <Button
              variant="secondary"
              size="compact"
              label={m('voice.join_call')}
              onclick={() => onjoincall(call.roomId)}
            >
              <span aria-hidden="true" class="iconify icon-[uil--phone]"></span>
              {m('room.directory.join')}
            </Button>
          </li>
        {/each}
      </ul>
    {/if}

    {#if catchUp.length > 0}
      <div data-testid="server-overview-catch-up">
        {#if calls.length > 0}
          <h3 class="px-3 pt-3 pb-1 section-label text-muted">
            {m('chat.overview.catch_up')}
          </h3>
        {/if}
        <ul class="selectable-list">
          {#each catchUp as room (room.id)}
            <!-- The link opens the room; each badge opens its newest notification, as in the sidebar. -->
            <li class="flex min-w-0 items-center gap-1 selectable-list-item pe-2">
              <a
                class="flex min-w-0 flex-1 items-center gap-2 py-1.5 ps-3"
                href={roomHref(room.id)}
              >
                <span aria-hidden="true" class="shrink-0 text-muted/60">#</span>
                <bdi class="min-w-0 flex-1 truncate font-medium">{room.name}</bdi>
                {#if room.importantCount === 0 && room.otherCount === 0}
                  <!-- The badge buttons' 24 px slot, so dots and badges share a centre line. -->
                  <span class="flex size-6 shrink-0 items-center justify-center">
                    <UnreadDot color="neutral" />
                  </span>
                  <span class="sr-only">{m('chat.overview.catch_up_unread')}</span>
                {/if}
              </a>
              <RoomAttentionBadges
                roomId={room.id}
                importantCount={room.importantCount}
                otherCount={room.otherCount}
                onopen={onopennotification}
              />
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </Panel>
{/if}
