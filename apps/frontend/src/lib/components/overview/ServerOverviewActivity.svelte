<!--
@component

The "Happening now" column of the server Overview: a card for each active
call in a joined room, and the joined rooms with unread messages or
notifications, stacked for the narrow side column.
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
  import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
  import { NotificationBadge, Panel, UnreadDot } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import OverviewSection from './OverviewSection.svelte';

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

  /** Avatars shown for each call; the count beside them includes everyone. */
  const VISIBLE_PARTICIPANTS = 4;

  function roomHref(roomId: string) {
    return resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId });
  }
</script>

{#snippet badge(roomId: string, attentionLevel: NotificationAttentionLevel, count: number)}
  {@const important = attentionLevel === NotificationAttentionLevel.IMPORTANT}
  <button
    type="button"
    class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
    aria-label={important
      ? m('room_list.go_to_notifications', { count })
      : m('room_list.go_to_other_notifications', { count })}
    onclick={() => onopennotification(roomId, attentionLevel)}
  >
    <NotificationBadge {count} color={important ? 'warning' : 'ambient'} />
  </button>
{/snippet}

{#if calls.length > 0 || catchUp.length > 0}
  <OverviewSection title={m('chat.overview.happening_now')}>
    <!-- One column in the side column; two when the stacked layout gives it the width. -->
    <div class="grid grid-cols-1 items-start gap-4 @xl:grid-cols-2">
      {#each calls as call (call.roomId)}
        <!-- One row: a live icon, the room with its participants, and the join action. -->
        <div data-testid="server-overview-call">
          <Panel>
            <div class="flex min-w-0 items-center gap-3">
              <span
                class="flex size-10 shrink-0 items-center justify-center rounded-full bg-success/15 text-success"
                aria-hidden="true"
              >
                <span aria-hidden="true" class="iconify icon-[uil--volume] size-5"></span>
              </span>
              <div class="flex min-w-0 flex-1 flex-col gap-1">
                <span class="truncate font-semibold text-text-top"><bdi>#{call.roomName}</bdi></span
                >
                <div class="flex min-w-0 items-center gap-2">
                  <div class="flex shrink-0 -space-x-1" aria-hidden="true">
                    {#each call.participants.slice(0, VISIBLE_PARTICIPANTS) as participant (participant.userId)}
                      <span class="inline-flex rounded-full ring-2 ring-surface">
                        <UserAvatar
                          user={callParticipantAvatarUser(participant)}
                          size="xs"
                          useLiveProfile={false}
                        />
                      </span>
                    {/each}
                  </div>
                  <span class="truncate text-muted">
                    {m('chat.overview.call_participants_count', {
                      count: call.participants.length
                    })}
                  </span>
                </div>
              </div>
              <!-- The short visible label is part of the accessible name, which names the call. -->
              <Button
                size="sm"
                label={m('voice.join_call')}
                onclick={() => onjoincall(call.roomId)}
              >
                <span aria-hidden="true" class="iconify icon-[uil--phone]"></span>
                {m('room.directory.join')}
              </Button>
            </div>
          </Panel>
        </div>
      {/each}

      {#if catchUp.length > 0}
        <div data-testid="server-overview-catch-up">
          <Panel title={m('chat.overview.catch_up')} noPadding>
            <ul class="selectable-list">
              {#each catchUp as room (room.id)}
                <!-- The link opens the room; each badge opens its newest notification, as in the sidebar. -->
                <li class="flex min-w-0 items-center gap-1 selectable-list-item pe-3">
                  <a
                    class="flex min-w-0 flex-1 items-center gap-2 py-1.5 ps-3"
                    href={roomHref(room.id)}
                  >
                    <span class="shrink-0 text-muted/60" aria-hidden="true">#</span>
                    <bdi class="min-w-0 flex-1 truncate font-medium">{room.name}</bdi>
                    {#if room.importantCount === 0 && room.otherCount === 0}
                      <UnreadDot color="neutral" />
                      <span class="sr-only">{m('chat.overview.catch_up_unread')}</span>
                    {/if}
                  </a>
                  {#if room.otherCount > 0}
                    {@render badge(room.id, NotificationAttentionLevel.AMBIENT, room.otherCount)}
                  {/if}
                  {#if room.importantCount > 0}
                    {@render badge(
                      room.id,
                      NotificationAttentionLevel.IMPORTANT,
                      room.importantCount
                    )}
                  {/if}
                </li>
              {/each}
            </ul>
          </Panel>
        </div>
      {/if}
    </div>
  </OverviewSection>
{/if}
