<!--
@component

Floating typing indicator that appears in the lower inline-end corner of a room
or thread pane. Shows avatars of typing users together with a compact textual
label that names up to two people and aggregates larger groups
("A, B and 3 others are typing").

The indicator is positioned absolutely so its appearance never shifts the
message list layout, and it announces changes politely to screen readers via a
`role="status"` region.

**Props:**
- `typingUserIds` - Array of user IDs currently typing
- `members` - Room members for resolving avatars and display names
-->
<script module lang="ts">
  /** Maximum number of avatars shown regardless of group size. */
  export const MAX_TYPING_AVATARS = 3;

  /** Maximum number of names spelled out in the label before aggregating. */
  const MAX_LABEL_NAMES = 2;
</script>

<script lang="ts">
  import { fade } from 'svelte/transition';
  import { type RoomMember } from '$lib/state/room';
  import { m } from '$lib/i18n/messages';
  import { expoOutTransition } from '$lib/ui/motion';
  import UserAvatar from '$lib/components/UserAvatar.svelte';

  let {
    typingUserIds,
    members
  }: {
    typingUserIds: string[];
    members: RoomMember[];
  } = $props();

  // Resolve user IDs to members (for avatar URLs and display names), keeping
  // the order in which typers were reported.
  let activeUserIds = $derived([...new Set(typingUserIds)]);
  let typingMembers = $derived(
    activeUserIds
      .map((id) => members.find((member) => member.id === id))
      .filter((member): member is RoomMember => member != null)
  );

  let visibleMembers = $derived(typingMembers.slice(0, MAX_TYPING_AVATARS));

  /**
   * Compact human label: one name, two names, or an aggregate fallback for
   * larger groups. Missing profiles use the localized unknown-user label.
   * Isolate user-authored names so their direction cannot reorder the label.
   */
  let label = $derived.by(() => {
    if (activeUserIds.length === 0) return '';

    const names = activeUserIds.slice(0, MAX_LABEL_NAMES).map((id) => {
      const member = members.find((member) => member.id === id);
      const name = member?.displayName || member?.login || m('common.unknown_user');
      return String.fromCodePoint(0x2068) + name + String.fromCodePoint(0x2069);
    });

    if (names.length === 1) {
      return m('room.typing.one', { name: names[0] });
    }

    if (activeUserIds.length === 2) {
      return m('room.typing.two', { first: names[0], second: names[1] });
    }

    const otherCount = activeUserIds.length - MAX_LABEL_NAMES;
    return m('room.typing.many_count', { count: otherCount, names: names.join(', ') });
  });
</script>

<!-- Keep the live region mounted before text arrives so updates can be announced. -->
<div
  role="status"
  aria-live="polite"
  aria-atomic="true"
  class="pointer-events-none absolute inset-x-2 bottom-0 z-10 flex justify-end"
>
  {#if activeUserIds.length > 0}
    <div
      data-testid="typing-indicator"
      class="flex max-w-full min-w-0 items-center gap-1.5 rounded-md border border-border bg-background px-2 py-1 shadow-md"
      transition:fade={expoOutTransition(150)}
    >
      {#each visibleMembers as member (member.id)}
        <span class="shrink-0" aria-hidden="true" data-testid="typing-avatar">
          <UserAvatar user={member} size="xs" useLiveProfile={false} />
        </span>
      {/each}
      {#if label}
        <span class="typing-label ms-0.5 max-w-64 min-w-0 truncate text-muted">{label}</span>
      {/if}
      <span class="typing-dots inline-flex shrink-0 items-center gap-0.5" aria-hidden="true">
        <span class="typing-dot"></span>
        <span class="typing-dot [animation-delay:200ms]"></span>
        <span class="typing-dot [animation-delay:400ms]"></span>
      </span>
    </div>
  {/if}
</div>
