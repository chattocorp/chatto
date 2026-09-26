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
- `profiles` - Shared profiles for typers whose room membership is still loading
-->
<script module lang="ts">
  /** Maximum number of avatars shown regardless of group size. */
  export const MAX_TYPING_AVATARS = 3;

  /** Maximum number of names spelled out in the label before aggregating. */
  const MAX_LABEL_NAMES = 2;
</script>

<script lang="ts">
  import { accountNameToken } from '$lib/render/accountName';
  import { scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { prefersReducedMotion } from 'svelte/motion';
  import { type RoomMember } from '$lib/state/room';
  import { m } from '$lib/i18n/messages';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { mapDirectoryMember } from '$lib/api-client/directoryMemberView';
  import type { UserStore } from '$lib/state/server/users.svelte';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';

  let {
    typingUserIds,
    members,
    profiles
  }: {
    typingUserIds: string[];
    members: RoomMember[];
    /** Shared profiles can name a typer before room membership finishes loading. */
    profiles?: Pick<UserStore, 'get' | 'isDeleted'>;
  } = $props();

  function resolveMember(id: string): RoomMember | undefined {
    if (profiles?.isDeleted(id)) return undefined;
    const profile = profiles?.get(id);
    return profile ? mapDirectoryMember(profile) : members.find((member) => member.id === id);
  }

  // Resolve user IDs to members (for avatar URLs and display names), keeping
  // the order in which typers were reported.
  let activeUserIds = $derived([...new Set(typingUserIds)]);
  let typingMembers = $derived(
    activeUserIds.map(resolveMember).filter((member): member is RoomMember => member != null)
  );

  let visibleMembers = $derived(typingMembers.slice(0, MAX_TYPING_AVATARS));

  /**
   * Compact human label: one name, two names, or an aggregate fallback for
   * larger groups. Missing profiles use the localized unknown-user label.
   * Isolate user-authored names so their direction cannot reorder the label.
   */
  let label = $derived.by(() => {
    if (activeUserIds.length === 0) return '';

    const names = activeUserIds
      .slice(0, MAX_LABEL_NAMES)
      .map((_, index) => accountNameToken(index));

    if (names.length === 1) {
      return m('room.typing.one', { name: names[0] });
    }

    if (activeUserIds.length === 2) {
      return m('room.typing.two', { first: names[0], second: names[1] });
    }

    const otherCount = activeUserIds.length - MAX_LABEL_NAMES;
    return m('room.typing.many_count', { count: otherCount, names: names.join(', ') });
  });
  let labelAccounts = $derived(
    activeUserIds.slice(0, MAX_LABEL_NAMES).map((id) => {
      const member = resolveMember(id);
      return {
        name: member?.displayName || member?.login || m('common.unknown_user'),
        identity: member
      };
    })
  );
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
      transition:scale={{
        duration: prefersReducedMotion.current ? 0 : 150,
        easing: cubicOut,
        start: 0.96,
        opacity: 0
      }}
    >
      {#each visibleMembers as member (member.id)}
        <span class="flex shrink-0 items-center" aria-hidden="true" data-testid="typing-avatar">
          <UserAvatar user={member} size="xs" useLiveProfile={false} />
        </span>
      {/each}
      {#if label}
        <span class="typing-label ms-0.5 max-w-64 min-w-0 truncate text-muted">
          <AccountNameTokens text={label} accounts={labelAccounts} />
        </span>
      {/if}
      <span class="typing-dots grid shrink-0 grid-cols-3 gap-0.5 text-muted" aria-hidden="true">
        <!-- Clockwise perimeter chase; negative delays start with a complete fading trail. -->
        <span class="typing-dot"></span>
        <span class="typing-dot [animation-delay:-700ms]"></span>
        <span class="typing-dot [animation-delay:-600ms]"></span>
        <span class="typing-dot [animation-delay:-100ms]"></span>
        <span class="typing-dot [animation:none] opacity-20"></span>
        <span class="typing-dot [animation-delay:-500ms]"></span>
        <span class="typing-dot [animation-delay:-200ms]"></span>
        <span class="typing-dot [animation-delay:-300ms]"></span>
        <span class="typing-dot [animation-delay:-400ms]"></span>
      </span>
    </div>
  {/if}
</div>
