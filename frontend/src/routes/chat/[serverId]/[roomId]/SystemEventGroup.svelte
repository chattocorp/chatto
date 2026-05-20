<script lang="ts">
  import type { RoomEventViewFragment, UserAvatarUserFragment } from '$lib/gql/graphql';
  import type { SystemGroupKind } from './virtualItems';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import { useFragment } from '$lib/gql/fragment-masking';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';

  let {
    events,
    kind
  }: {
    events: RoomEventViewFragment[];
    kind: SystemGroupKind;
  } = $props();

  const action = $derived(kind === 'join' ? 'joined the room' : 'left the room');

  type Actor = {
    id: string;
    name: string;
    user: UserAvatarUserFragment | null;
  };

  // Deduplicate by actor so a user appearing twice in a row (rare, but possible)
  // doesn't get listed twice. Preserves first-seen order.
  const actors = $derived.by<Actor[]>(() => {
    const result: Actor[] = [];
    for (const event of events) {
      const user = event?.actor ? useFragment(UserAvatarFragment, event.actor) : null;
      const id = user?.id ?? `__deleted_${event.actorId ?? 'unknown'}`;
      if (result.some((a) => a.id === id)) continue;
      const name = user
        ? getLiveDisplayName(user.id, user.displayName || user.login)
        : 'Deleted User';
      result.push({ id, name, user });
    }
    return result;
  });

  const MAX_AVATARS = 3;
  const NAMES_BEFORE_TRUNCATION = 3;
  const visibleAvatars = $derived(actors.slice(0, MAX_AVATARS));
  const isTruncatable = $derived(actors.length > NAMES_BEFORE_TRUNCATION + 1);

  let expanded = $state(false);

  function joinNames(names: string[]): string {
    if (names.length === 0) return '';
    if (names.length === 1) return names[0];
    if (names.length === 2) return `${names[0]} and ${names[1]}`;
    return `${names.slice(0, -1).join(', ')}, and ${names[names.length - 1]}`;
  }

  const allNames = $derived(joinNames(actors.map((a) => a.name)));
  const headNames = $derived(actors.slice(0, NAMES_BEFORE_TRUNCATION).map((a) => a.name).join(', '));
  const extraCount = $derived(Math.max(actors.length - NAMES_BEFORE_TRUNCATION, 0));
</script>

{#if actors.length > 0}
  <div class="mt-4 flex items-center gap-4 px-2 md:px-4">
    <!-- Avatar column (w-11 matches MessageEvent avatar width) -->
    <div class="flex w-11 shrink-0 items-center justify-center">
      <div class="flex -space-x-1.5">
        {#each visibleAvatars as actor (actor.id)}
          {#if actor.user}
            <UserAvatar user={actor.user} size="xs" showPresence={false} />
          {:else}
            <div
              class="flex h-5 w-5 items-center justify-center rounded-full bg-surface-200 text-muted ring-1 ring-background"
            >
              <span class="iconify text-xs uil--user-times"></span>
            </div>
          {/if}
        {/each}
      </div>
    </div>

    <span class="text-sm text-muted">
      {#if !isTruncatable || expanded}
        {allNames} {action}
        {#if isTruncatable}
          <button
            type="button"
            class="ml-1 cursor-pointer underline decoration-dotted underline-offset-2 hover:text-text"
            onclick={() => (expanded = false)}
          >
            show less
          </button>
        {/if}
      {:else}
        {headNames}, and <button
          type="button"
          class="cursor-pointer underline decoration-dotted underline-offset-2 hover:text-text"
          onclick={() => (expanded = true)}
        >{extraCount} {extraCount === 1 ? 'other' : 'others'}</button>
        {action}
      {/if}
    </span>
  </div>
{/if}
