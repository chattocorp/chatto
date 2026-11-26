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
  import { NotificationBadge, Panel, UnreadDot } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import OverviewSection from './OverviewSection.svelte';

  let {
    serverSegment,
    calls,
    catchUp,
    onjoincall
  }: {
    serverSegment: string;
    calls: readonly OverviewCall[];
    catchUp: readonly CatchUpRoom[];
    /** Open the call of a room. */
    onjoincall: (roomId: string) => void;
  } = $props();

  /** Avatars shown for each call; the count beside them includes everyone. */
  const VISIBLE_PARTICIPANTS = 5;

  function roomHref(roomId: string) {
    return resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId });
  }
</script>

{#if calls.length > 0 || catchUp.length > 0}
  <OverviewSection title={m('chat.overview.happening_now')}>
    <!-- One column in the side column; two when the stacked layout gives it the width. -->
    <div class="grid items-start gap-4 @xl:grid-cols-2">
      {#each calls as call (call.roomId)}
        <div data-testid="server-overview-call">
          <Panel>
            <div class="flex flex-col gap-3">
              <div class="flex min-w-0 items-center gap-2 font-semibold text-text-top">
                <span class="size-2 shrink-0 rounded-full bg-success" aria-hidden="true"></span>
                <a class="truncate link" href={roomHref(call.roomId)}><bdi>#{call.roomName}</bdi></a
                >
              </div>
              <div class="flex min-w-0 items-center gap-3">
                <div class="flex shrink-0 -space-x-1.5" aria-hidden="true">
                  {#each call.participants.slice(0, VISIBLE_PARTICIPANTS) as participant (participant.userId)}
                    <span class="inline-flex rounded-full ring-2 ring-surface">
                      <UserAvatar
                        user={callParticipantAvatarUser(participant)}
                        size="sm"
                        useLiveProfile={false}
                      />
                    </span>
                  {/each}
                </div>
                <span class="text-muted">
                  {m('chat.overview.call_participants_count', { count: call.participants.length })}
                </span>
              </div>
              <div>
                <Button size="sm" onclick={() => onjoincall(call.roomId)}>
                  <span aria-hidden="true" class="iconify icon-[uil--phone]"></span>
                  {m('voice.join_call')}
                </Button>
              </div>
            </div>
          </Panel>
        </div>
      {/each}

      {#if catchUp.length > 0}
        <div data-testid="server-overview-catch-up">
          <Panel title={m('chat.overview.catch_up')} noPadding>
            <ul class="selectable-list">
              {#each catchUp as room (room.id)}
                <li class="selectable-list-item">
                  <a class="flex min-w-0 items-center gap-2 px-3 py-1.5" href={roomHref(room.id)}>
                    <span class="shrink-0 text-muted/60" aria-hidden="true">#</span>
                    <bdi class="min-w-0 flex-1 truncate font-medium">{room.name}</bdi>
                    {#if room.otherCount > 0}
                      <NotificationBadge count={room.otherCount} color="ambient" />
                    {/if}
                    {#if room.importantCount > 0}
                      <NotificationBadge count={room.importantCount} color="warning" />
                    {/if}
                    {#if room.importantCount === 0 && room.otherCount === 0}
                      <UnreadDot color="neutral" />
                    {/if}
                    <span class="sr-only">
                      {room.importantCount + room.otherCount > 0
                        ? m('chat.overview.catch_up_notifications_count', {
                            count: room.importantCount + room.otherCount
                          })
                        : m('chat.overview.catch_up_unread')}
                    </span>
                  </a>
                </li>
              {/each}
            </ul>
          </Panel>
        </div>
      {/if}
    </div>
  </OverviewSection>
{/if}
