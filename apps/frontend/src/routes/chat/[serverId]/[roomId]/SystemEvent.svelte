<script lang="ts">
  import type { RoomEventView, UserAvatarUserView } from '$lib/render/types';
  import UserAvatar, { UserAvatarViewData } from '$lib/components/UserAvatar.svelte';
  import { useRenderData } from '$lib/render/data';
  import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import DeletedUserLabel from '$lib/components/DeletedUserLabel.svelte';
  import * as m from '$lib/i18n/messages';

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
    if (actor && !actor.deleted) {
      return { id: actor.id, name: displayName(actor), user: actor };
    }

    return { id: event?.actorId ?? 'unknown', name: 'Deleted User', user: null };
  });

  const eventKind = $derived(event?.event ? roomEventKind(event.event) : null);

  const action = $derived.by(() => {
    switch (eventKind) {
      case RoomEventKind.UserJoinedRoom:
        return m['room.system_events.joined']({ count: 1 });
      case RoomEventKind.UserLeftRoom:
        return m['room.system_events.left']({ count: 1 });
      case RoomEventKind.RoomArchived:
        return m['room.system_events.archived']();
      case RoomEventKind.RoomUnarchived:
        return m['room.system_events.unarchived']();
      default:
        return null;
    }
  });

  const isDeletedJoinLeave = $derived(
    !subject.user &&
      (eventKind === RoomEventKind.UserJoinedRoom || eventKind === RoomEventKind.UserLeftRoom)
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
          <span class="iconify text-xs uil--user-times"></span>
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
