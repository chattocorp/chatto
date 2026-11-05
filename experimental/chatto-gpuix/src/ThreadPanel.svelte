<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { mapDirectoryRoomDetails } from '@chatto/client/api/roomDirectory';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
  import type { Server } from '@chatto/client';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import MessageTimeline from './MessageTimeline.svelte';
  import Composer from './ui/Composer.svelte';
  import PaneHeader from './ui/PaneHeader.svelte';
  import IconButton from './ui/IconButton.svelte';
  let {
    server,
    roomId,
    rootId,
    compact,
    onClose
  }: {
    server: Server;
    roomId: string;
    rootId: string;
    compact: boolean;
    onClose: () => void;
  } = $props();
  const timeline = $derived.by(() => {
    const owner = server;
    const room = roomId;
    const root = rootId;
    return untrack(() => owner.rooms.thread(room, root));
  });
  const room = $derived(server.roomList.rooms.find((item) => item.id === roomId));
  const root = $derived(timeline.threadEvents.find((item) => item.id === rootId));
  const message = $derived(root && isMessagePostedEvent(root.event) ? root.event : null);
  const details = $derived(mapDirectoryRoomDetails(server.projection.rooms.get(roomId)));
  // Prefer the canonical root's authority; older responses use SDK room permissions.
  const canReply = $derived(
    timeline.isCurrent &&
      details?.threadingMode !== RoomThreadingMode.DISABLED &&
      !message?.deletedAt &&
      (message?.canReplyInThread ?? details?.canPostInThread ?? false)
  );
  let now = $state(Date.now());
  const deadline = $derived(timeline.threadSlowModeNextPostAt ?? message?.threadSlowModeNextPostAt);
  const waitSeconds = $derived(
    deadline ? Math.max(0, Math.ceil((Date.parse(deadline) - now) / 1000)) : 0
  );
  let draft = $state('');
  let sending = $state(false);
  let error = $state('');
  let alive = true;
  onDestroy(() => {
    alive = false;
  });
  async function send() {
    if (!canReply || waitSeconds > 0 || !draft.trim() || sending) return;
    const body = draft;
    sending = true;
    error = '';
    try {
      await server.createMessage({ roomId, threadRootId: rootId }, body);
      if (alive && draft === body) draft = '';
    } catch {
      if (alive) error = 'The reply could not be confirmed. Check the thread before trying again.';
    } finally {
      if (alive) sending = false;
    }
  }
</script>

<div
  class={['thread-panel', { compact }]}
  testid="thread-panel"
  {@attach () => {
    const timer = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(timer);
  }}
>
  <PaneHeader
    title="Thread"
    description={room?.name ?? 'Conversation'}
    icon="message"
    testid="thread-pane-header"
  >
    {#snippet actions()}
      <IconButton
        label={compact ? 'Back to channel' : 'Close thread'}
        icon={compact ? 'back' : 'close'}
        onclick={onClose}
        testid="thread-close"
      />
    {/snippet}
  </PaneHeader>
  <MessageTimeline {server} {timeline} scope={`${roomId}:${rootId}`} thread idPrefix="thread" />
  {#if !timeline.isInitialLoading && !canReply}<text class="notice"
      >Replies are not available in this thread.</text
    >{/if}
  {#if waitSeconds > 0}<text class="notice">Wait {waitSeconds}s before replying.</text>{/if}
  <Composer
    {draft}
    {sending}
    canSend={canReply && waitSeconds === 0 && !!draft.trim()}
    onChange={(value) => (draft = value)}
    onSend={() => void send()}
    idPrefix="thread"
    placeholder="Reply to thread…"
    label="Thread reply"
  />
  {#if error}<text class="error">{error}</text>{/if}
</div>

<style>
  .thread-panel {
    display: flex;
    flex-direction: column;
    width: var(--size-430);
    max-width: 50%;
    height: 100%;
    flex-shrink: 0;
    border-left-width: var(--size-1);
    border-color: var(--color-border);
    background-color: var(--color-background);
  }
  .compact {
    width: auto;
    max-width: 100%;
    min-width: 0;
    flex-grow: 1;
  }
  .notice {
    font-size: var(--text-small);
    color: var(--color-muted);
    padding: var(--size-8) var(--size-12);
    white-space: normal;
  }
  .error {
    font-size: var(--text-small);
    color: var(--color-error);
    padding: var(--size-8) var(--size-12);
    white-space: normal;
  }
</style>
