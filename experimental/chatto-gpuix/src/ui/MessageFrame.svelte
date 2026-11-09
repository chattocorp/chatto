<script lang="ts">
  import type { Snippet } from 'svelte';
  import Avatar from './Avatar.svelte';
  let {
    author,
    time,
    bot = false,
    avatarUrl,
    serverUrl,
    active = false,
    children
  }: {
    author: string;
    time: string;
    bot?: boolean;
    avatarUrl?: string | null;
    serverUrl?: string;
    active?: boolean;
    children: Snippet;
  } = $props();
</script>

<div class="message-row">
  <Avatar name={author} src={avatarUrl} {serverUrl} {active} />
  <div class="content">
    <div class="byline">
      <text class="author">{author}</text>{#if bot}<text class="badge">BOT</text>{/if}<text
        class="time">{time}</text
      >
    </div>
    {@render children()}
  </div>
</div>

<style>
  .message-row {
    display: flex;
    flex-direction: row;
    align-items: flex-start;
    gap: var(--size-13);
    padding: var(--size-12) var(--size-18);
    min-height: var(--size-60);
  }
  .message-row:hover {
    background-color: #1b1b1b;
  }
  .content {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-width: 0;
    gap: var(--size-6);
  }
  .byline {
    display: flex;
    align-items: center;
    gap: var(--size-8);
  }
  .author {
    font-size: var(--text-body);
    font-weight: 600;
    color: var(--color-text-top);
  }
  .time {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .badge {
    font-size: var(--text-badge);
    font-weight: 600;
    color: var(--color-text);
    background-color: var(--color-surface-emphasized);
    border-radius: var(--size-3);
    padding: var(--size-1) var(--size-4);
  }
</style>
