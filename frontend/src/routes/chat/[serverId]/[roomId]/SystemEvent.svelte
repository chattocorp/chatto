<script lang="ts">
  import type { RoomEventViewFragment } from '$lib/gql/graphql';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import { useFragment } from '$lib/gql/fragment-masking';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';

  let { event }: { event: RoomEventViewFragment } = $props();

  // Actor may be null if the user has been deleted.
  // Guard with event?. for Svelte 5 reactivity glitch during virtualizer data transitions.
  const actor = $derived(event?.actor ? useFragment(UserAvatarFragment, event.actor) : null);
  const actorName = $derived(
    actor ? getLiveDisplayName(actor.id, actor.displayName || actor.login) : 'Deleted User'
  );

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
</script>

{#if action}
  <div class="mt-4 flex items-center gap-4 px-2 md:px-4">
    <!-- Avatar column (w-11 matches MessageEvent avatar width) -->
    <div class="flex w-11 shrink-0 items-center justify-center">
      {#if actor}
        <UserAvatar user={actor} size="xs" showPresence={false} />
      {:else}
        <!-- Deleted user placeholder -->
        <div
          class="flex h-5 w-5 items-center justify-center rounded-full bg-surface-200 text-muted"
        >
          <span class="iconify text-xs uil--user-times"></span>
        </div>
      {/if}
    </div>

    <span class="text-sm text-muted">{actorName} {action}</span>
  </div>
{/if}
