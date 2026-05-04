<!--
@component

Floating typing indicator that appears in the lower-right corner of a room
or thread pane. Shows small avatars of typing users with animated dots.

**Props:**
- `typingUserIds` - Array of user IDs currently typing
- `members` - Room members for resolving avatars
-->
<script lang="ts">
  import { fade } from 'svelte/transition';
  import { type RoomMember } from '$lib/state/room';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

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
    class="pointer-events-none absolute right-2 bottom-0 z-10 flex items-center gap-1.5 rounded-md border border-border bg-background px-2 py-1 shadow-md"
    transition:fade={{ duration: 150 }}
  >
    {#each typingMembers as member (member.id)}
      {#if member.avatarUrl}
        <SkeletonImg src={member.avatarUrl} alt={member.displayName} class="size-5 rounded-full" />
      {:else}
        <div
          class="flex size-5 items-center justify-center rounded-full bg-muted/20 text-[8px] font-medium text-muted"
        >
          {member.displayName?.charAt(0).toUpperCase() ?? '?'}
        </div>
      {/if}
    {/each}
    <span class="typing-dots ml-0.5">
      <span class="dot"></span>
      <span class="dot"></span>
      <span class="dot"></span>
    </span>
  </div>
{/if}

<style>
  .typing-dots {
    display: inline-flex;
    gap: 2px;
    align-items: center;
  }

  .dot {
    width: 3px;
    height: 3px;
    background-color: currentColor;
    border-radius: 50%;
    opacity: 0.5;
    animation: typing-bounce 1.4s ease-in-out infinite;
  }

  .dot:nth-child(1) {
    animation-delay: 0s;
  }

  .dot:nth-child(2) {
    animation-delay: 0.2s;
  }

  .dot:nth-child(3) {
    animation-delay: 0.4s;
  }

  @keyframes typing-bounce {
    0%,
    60%,
    100% {
      transform: translateY(0);
      opacity: 0.4;
    }
    30% {
      transform: translateY(-3px);
      opacity: 1;
    }
  }
</style>
