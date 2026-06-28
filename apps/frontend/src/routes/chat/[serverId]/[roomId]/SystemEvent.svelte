<script lang="ts">
  import type { RoomEventView, UserAvatarUserView } from '$lib/render/types';
  import UserAvatar, { UserAvatarViewData } from '$lib/components/UserAvatar.svelte';
  import { useRenderData } from '$lib/render/data';
  import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';

  let { event }: { event: RoomEventView } = $props();

  type Subject = {
    id: string;
    name: string;
    user: UserAvatarUserView | null;
  };

  function displayName(user: UserAvatarUserView): string {
    return getLiveDisplayName(user.id, user.displayName || user.login);
  }

  const subject = $derived.by<Subject>(() => {
    const actor = event?.actor ? useRenderData(UserAvatarViewData, event.actor) : null;
    if (actor) {
      return { id: actor.id, name: displayName(actor), user: actor };
    }

    return { id: event?.actorId ?? 'unknown', name: 'Deleted User', user: null };
  });

  const action = $derived.by(() => {
    if (!event?.event) return null;
    switch (roomEventKind(event.event)) {
      case RoomEventKind.UserJoinedRoom:
        return 'joined the room';
      case RoomEventKind.UserLeftRoom:
        return 'left the room';
      case RoomEventKind.RoomArchived:
        return 'archived the room';
      case RoomEventKind.RoomUnarchived:
        return 'unarchived the room';
      default:
        return null;
    }
  });

  const message = $derived.by(() => {
    if (!action) return null;
    return `${subject.name} ${action}`;
  });
</script>

{#if message}
  <div class="mt-4 flex items-center gap-4 px-2 md:px-4" data-event-id={event.id}>
    <!-- Avatar column (w-11 matches MessageEvent avatar width) -->
    <div class="flex w-11 shrink-0 items-center justify-center">
      {#if subject.user}
        <UserAvatar user={subject.user} size="xs" />
      {:else}
        <!-- Deleted user placeholder -->
        <div
          class="flex h-5 w-5 items-center justify-center rounded-full bg-surface-200 text-muted"
        >
          <span class="iconify text-xs uil--user-times"></span>
        </div>
      {/if}
    </div>

    <span class="text-sm text-muted">{message}</span>
  </div>
{/if}
