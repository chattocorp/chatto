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
    gap: 13px;
    padding: 12px 18px;
    min-height: 60px;
  }
  .message-row:hover {
    background-color: #1b1b1b;
  }
  .content {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-width: 0;
    gap: 6px;
  }
  .byline {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .author {
    font-size: 14px;
    font-weight: 600;
    color: var(--color-text-top);
  }
  .time {
    font-size: 11px;
    color: var(--color-muted);
  }
  .badge {
    font-size: 10px;
    font-weight: 600;
    color: var(--color-text);
    background-color: var(--color-surface-emphasized);
    border-radius: 3px;
    padding: 1px 4px;
  }
</style>
