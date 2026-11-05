<script lang="ts">
  import type { Server } from '@chatto/client';
  import ActionButton from './ActionButton.svelte';
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
    onOpenThread,
    dayLabel = null
  }: {
    item: TimelineEventView;
    server: Server;
    active: boolean;
    dayLabel?: string | null;
    onOpenThread?: (id: string) => void;
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
      <markdown source={markdown} theme={markdownTheme()} class="body"></markdown>
    {/if}
    {#each message?.attachments ?? [] as attachment (attachment.id)}
      <Attachment {attachment} {server} roomId={message!.roomId} {active} />
    {/each}
    {#if message?.linkPreview}
      <LinkPreview preview={message.linkPreview} serverUrl={server.serverUrl} {active} />
    {/if}
    {#if message && onOpenThread}
      <div class="thread-action">
        <ActionButton
          label={message.replyCount
            ? `${message.replyCount} ${message.replyCount === 1 ? 'reply' : 'replies'}`
            : 'Reply in thread'}
          icon="message"
          quiet
          disabled={message.canReplyInThread === false &&
            !message.threadExists &&
            !message.replyCount}
          onclick={() =>
            onOpenThread?.(
              message.echoFromThreadRootEventId ||
                message.threadRootEventId ||
                message.echoOfEventId ||
                item.id
            )}
          testid={`thread-open-${item.id}`}
        />
      </div>
    {/if}
  </MessageFrame>
</div>

<style>
  .thread-action {
    display: flex;
    flex-direction: row;
  }
  .entry {
    display: flex;
    flex-direction: column;
    width: 100%;
  }
  .body {
    font-size: var(--text-body);
    color: var(--color-text);
    white-space: normal;
    user-select: text;
  }
</style>
