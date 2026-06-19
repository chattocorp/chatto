<script lang="ts">
  import type { RoomEventViewFragment, UserAvatarUserFragment } from '$lib/gql/graphql';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import { useFragment } from '$lib/gql/fragment-masking';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';

  let { event }: { event: RoomEventViewFragment } = $props();

  type Subject = {
    id: string;
    name: string;
    user: UserAvatarUserFragment | null;
  };

  function displayName(user: UserAvatarUserFragment): string {
    return getLiveDisplayName(user.id, user.displayName || user.login);
  }

  const subject = $derived.by<Subject>(() => {
    const actor = event?.actor ? useFragment(UserAvatarFragment, event.actor) : null;
    if (actor) {
      return { id: actor.id, name: displayName(actor), user: actor };
    }

    return { id: event?.actorId ?? 'unknown', name: 'Deleted User', user: null };
  });

  const action = $derived.by(() => {
    if (!event?.event) return null;
    switch (event.event.__typename) {
      case 'UserJoinedRoomEvent':
        return 'joined the room';
      case 'UserLeftRoomEvent':
        return 'left the room';
      case 'RoomArchivedEvent':
        return 'archived the room';
      case 'RoomUnarchivedEvent':
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
        <UserAvatar user={subject.user} size="xs" showPresence={false} />
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
