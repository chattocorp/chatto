<!--
@component

Floating typing indicator that appears in the lower inline-end corner of a room
or thread pane. Shows small avatars of typing users with animated dots.

**Props:**
- `typingUserIds` - Array of user IDs currently typing
- `members` - Room members for resolving avatars
-->
<script lang="ts">
  import { fade } from 'svelte/transition';
  import { type RoomMember } from '$lib/state/room';
  import UserAvatar from '$lib/components/UserAvatar.svelte';

  let {
    typingUserIds,
    members
  }: {
    typingUserIds: string[];
    members: RoomMember[];
  } = $props();

  // Resolve user IDs to members (for avatar URLs and names for alt text)
  let typingMembers = $derived(
    typingUserIds
      .map((id) => members.find((m) => m.id === id))
      .filter((m): m is RoomMember => m != null)
      .slice(0, 3)
  );
</script>

{#if typingUserIds.length > 0}
  <div
    class="pointer-events-none absolute end-2 bottom-0 z-10 flex items-center gap-1.5 rounded-md border border-border bg-background px-2 py-1 shadow-md"
    transition:fade={{ duration: 150 }}
  >
    {#each typingMembers as member (member.id)}
      <UserAvatar user={member} size="xs" useLiveProfile={false} />
    {/each}
    <span class="typing-dots ms-0.5 inline-flex items-center gap-0.5">
      <span class="typing-dot"></span>
      <span class="typing-dot [animation-delay:200ms]"></span>
      <span class="typing-dot [animation-delay:400ms]"></span>
    </span>
  </div>
{/if}
