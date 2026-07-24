<script lang="ts">
  import type { RoomEventView, UserAvatarUserView } from '$lib/render/types';
  import type { SystemGroupKind } from './virtualItems';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import * as m from '$lib/i18n/messages';

  let {
    events,
    kind,
    expanded,
    onExpandedChange
  }: {
    events: RoomEventView[];
    kind: SystemGroupKind;
    expanded: boolean;
    onExpandedChange: (expanded: boolean) => void;
  } = $props();

  const actionKind = $derived.by(() => {
    switch (kind) {
      case 'join':
        return 'joined';
      case 'leave':
        return 'left';
    }
  });

  type Actor = {
    id: string;
    name: string;
    user: UserAvatarUserView;
  };

  function displayName(user: UserAvatarUserView): string {
    return getLiveDisplayName(user.id, user.displayName || user.login);
  }

  function eventSubject(event: RoomEventView): Actor | null {
    const actor = event.actor ?? null;
    if (actor && !actor.deleted) {
      return { id: actor.id, name: displayName(actor), user: actor };
    }

    return null;
  }

  // Deleted actors add no useful membership context. Filter them before
  // deduplicating so names, avatars, counts, and truncation stay consistent.
  const actors = $derived.by<Actor[]>(() => {
    const result: Actor[] = [];
    for (const event of events) {
      const subject = eventSubject(event);
      if (!subject) continue;
      if (result.some((a) => a.id === subject.id)) continue;
      result.push(subject);
    }
    return result;
  });

  const MAX_AVATARS = 3;
  const NAMES_BEFORE_TRUNCATION = 3;
  const visibleAvatars = $derived(actors.slice(0, MAX_AVATARS));
  const isTruncatable = $derived(actors.length > NAMES_BEFORE_TRUNCATION + 1);

  const headActors = $derived(actors.slice(0, NAMES_BEFORE_TRUNCATION));
  const extraCount = $derived(Math.max(actors.length - NAMES_BEFORE_TRUNCATION, 0));
  const action = $derived(
    actionKind === 'joined'
      ? m['room.system_events.joined']({ count: actors.length })
      : m['room.system_events.left']({ count: actors.length })
  );
</script>

{#snippet actorName(actor: Actor)}
  {actor.name}
{/snippet}

{#snippet actorNames(items: Actor[])}
  {#each items as actor, index (actor.id)}
    {#if index > 0}
      {#if index === items.length - 1}
        {items.length > 2 ? ', ' : ' '}{m['room.system_events.and']()}
      {:else}
        ,
      {/if}
    {/if}
    {@render actorName(actor)}
  {/each}
{/snippet}

{#if actors.length > 0}
  <div class="mt-4 flex items-center gap-4 px-2 md:px-4" data-event-id={events[0].id}>
    <!-- Avatar column (w-11 matches MessageEvent avatar width) -->
    <div class="flex w-11 shrink-0 items-center justify-center">
      <div class="flex -space-x-1.5">
        {#each visibleAvatars as actor (actor.id)}
          <UserAvatar user={actor.user} size="xs" />
        {/each}
      </div>
    </div>

    <span class="text-sm text-muted">
      {#if !isTruncatable || expanded}
        {@render actorNames(actors)}
        {action}
        {#if isTruncatable}
          <button
            type="button"
            class="ml-1 cursor-pointer underline decoration-dotted underline-offset-2 hover:text-text"
            onclick={() => onExpandedChange(false)}
          >
            {m['room.system_events.show_less']()}
          </button>
        {/if}
      {:else}
        {@render actorNames(headActors)}, {m['room.system_events.and']()}
        <button
          type="button"
          class="cursor-pointer underline decoration-dotted underline-offset-2 hover:text-text"
          onclick={() => onExpandedChange(true)}
          >{extraCount} {m['room.system_events.other_people']({ count: extraCount })}</button
        >
        {action}
      {/if}
    </span>
  </div>
{/if}
