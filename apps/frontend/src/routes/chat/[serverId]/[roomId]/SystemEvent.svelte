<script lang="ts">
  import {
    TimelineEventKind,
    timelineEventKind,
    type TimelineEventView
  } from '$lib/render/timelineEvents';
  import type { UserAvatarUserView } from '$lib/render/users';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import DeletedUserLabel from '$lib/components/DeletedUserLabel.svelte';
  import { m } from '$lib/i18n/messages';

  let { event }: { event: TimelineEventView } = $props();

  type Subject = {
    id: string;
    name: string;
    user: UserAvatarUserView | null;
  };

  function displayName(user: UserAvatarUserView): string {
    return getLiveDisplayName(user.id, user.displayName || user.login);
  }

  const subject = $derived.by<Subject>(() => {
    const actor = event.actor ?? null;
    if (actor && !actor.deleted) {
      return { id: actor.id, name: displayName(actor), user: actor };
    }

    return { id: event?.actorId ?? 'unknown', name: 'Deleted User', user: null };
  });

  const eventKind = $derived(timelineEventKind(event.event));

  const action = $derived.by(() => {
    switch (eventKind) {
      case TimelineEventKind.UserJoinedRoom:
        return m('room.system_events.joined_count', { count: 1 });
      case TimelineEventKind.UserLeftRoom:
        return m('room.system_events.left_count', { count: 1 });
      case TimelineEventKind.RoomArchived:
        return m('room.system_events.archived');
      case TimelineEventKind.RoomUnarchived:
        return m('room.system_events.unarchived');
      default:
        return null;
    }
  });

  const isDeletedJoinLeave = $derived(
    !subject.user &&
      (eventKind === TimelineEventKind.UserJoinedRoom ||
        eventKind === TimelineEventKind.UserLeftRoom)
  );
</script>

{#if action && !isDeletedJoinLeave}
  <div class="mt-4 flex items-center gap-4 px-2 md:px-4" data-event-id={event.id}>
    <!-- Avatar column (w-11 matches MessageEvent avatar width) -->
    <div class="flex w-11 shrink-0 items-center justify-center">
      {#if subject.user}
        <UserAvatar user={subject.user} size="xs" />
      {:else}
        <!-- Deleted user placeholder -->
        <div
          class="flex h-5 w-5 items-center justify-center rounded-full bg-surface-emphasized text-muted"
        >
          <span class="iconify icon-[uil--user-times] text-xs"></span>
        </div>
      {/if}
    </div>

    <span class="text-sm text-muted">
      {#if subject.user}
        {subject.name}
      {:else}
        <DeletedUserLabel />
      {/if}
      {action}
    </span>
  </div>
{/if}
