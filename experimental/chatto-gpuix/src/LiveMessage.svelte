<script lang="ts">
  import type { Server } from '@chatto/client';
  import Attachment from './Attachment.svelte';
  import LinkPreview from './LinkPreview.svelte';
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import DateDivider from './ui/DateDivider.svelte';
  import { messageTime } from './ui/messageDates.ts';
  import MessageFrame from './ui/MessageFrame.svelte';
  import { markdownTheme } from './ui/theme.ts';
  import { safeMarkdown } from './markdown.ts';
  let {
    item,
    server,
    active,
    dayLabel = null
  }: {
    item: TimelineEventView;
    server: Server;
    active: boolean;
    dayLabel?: string | null;
  } = $props();
  const time = $derived(messageTime(item.createdAt));
  const message = $derived(isMessagePostedEvent(item.event) ? item.event : null);
  const body = $derived(isMessagePostedEvent(item.event) ? (item.event.body ?? '') : '');
  const markdown = $derived(safeMarkdown(body));
</script>

<!-- Keep a date divider and its message in one logical virtual-list row. -->
<div class="entry">
  {#if dayLabel}<DateDivider label={dayLabel} />{/if}
  <MessageFrame
    author={item.actor?.displayName || item.actor?.login || 'Deleted user'}
    {time}
    bot={item.actor?.isBot}
    avatarUrl={item.actor?.avatarUrl}
    serverUrl={server.serverUrl}
    {active}
  >
    {#if markdown}
      <markdown source={markdown} theme={markdownTheme} class="body"></markdown>
    {/if}
    {#each message?.attachments ?? [] as attachment (attachment.id)}
      <Attachment {attachment} {server} roomId={message!.roomId} {active} />
    {/each}
    {#if message?.linkPreview}
      <LinkPreview preview={message.linkPreview} serverUrl={server.serverUrl} {active} />
    {/if}
  </MessageFrame>
</div>

<style>
  .entry {
    display: flex;
    flex-direction: column;
    width: 100%;
  }
  .body {
    font-size: 14px;
    color: var(--color-text);
    white-space: normal;
    user-select: text;
  }
</style>
