<script lang="ts">
  /** Shared room/thread rendering. The parent keys this view by its SDK timeline identity. */
  import { flushSync, onDestroy } from 'svelte';
  import { commit, get_native, read_native, type ShadowNode } from 'gpuix-svelte';
  import type { Server } from '@chatto/client';
  import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import { appearance, scaled, beforeScaleChange } from './ui/scale.svelte.ts';
  import { messageDate, messageDay } from './ui/messageDates.ts';
  import ActionButton from './ActionButton.svelte';
  import LiveMessage from './LiveMessage.svelte';
  let {
    server,
    timeline,
    scope,
    thread = false,
    idPrefix = 'live',
    onOpenThread
  }: {
    server: Server;
    timeline: MessagesStore | null;
    scope: string;
    thread?: boolean;
    idPrefix?: string;
    onOpenThread?: (id: string) => void;
  } = $props();
  const events = $derived(
    (thread ? timeline?.threadEvents : timeline?.rootEvents)?.filter(
      (item) => isMessagePostedEvent(item.event) && !item.event.deletedAt
    ) ?? []
  );
  let historyNotice = $state('');
  let visible = $state({ start: -1, end: -1 });
  let listNode: ShadowNode | null = null;
  let zoomAnchor: { room: string; index: number; offset: number } | null = null;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });
  async function older(start: number) {
    if (
      start > 2 ||
      !timeline ||
      timeline.isInitialLoading ||
      timeline.isLoadingMore ||
      timeline.hasReachedStart
    )
      return;
    const owner = timeline;
    const node = listNode;
    const previousCount = owner.events.length;
    const previousFirst = owner.events[0]?.id;
    historyNotice = '';
    const offset =
      node?.nativeId == null
        ? null
        : read_native((native) => native.getListScrollTop?.(node.nativeId!));
    const anchor = offset ? events[offset[0] ?? 0]?.id : undefined;
    await owner.loadMore();
    flushSync();
    if (timeline !== owner || !alive) return;
    // The SDK logs page failures in the host terminal rather than rejecting.
    // An empty advancing page is also possible, so report only lack of rows.
    if (
      !owner.hasReachedStart &&
      owner.events.length === previousCount &&
      owner.events[0]?.id === previousFirst
    ) {
      historyNotice = 'No additional messages were loaded. Retry loading older messages.';
    }
    if (!anchor || !offset || !node || listNode !== node || timeline !== owner || !alive) return;
    const index = events.findIndex((event) => event.id === anchor);
    if (index < 0 || node.nativeId == null) return;
    // Commit the prepended rows first. Native applies this anchor after its
    // child splice, so a newly loaded page cannot move the visible message.
    commit();
    if (node.nativeId != null) get_native()?.scrollToItem?.(node.nativeId, index, offset[1] ?? 0);
  }
</script>

{#if timeline?.loadError}<text class="error">Could not load this conversation.</text><ActionButton
    label="Retry"
    onclick={() => void timeline?.restoreLatestWindow()}
  />{/if}
{#if timeline?.isInitialLoading || timeline?.isLoadingMore}<text class="muted status"
    >Loading messages…</text
  >{/if}
{#if historyNotice}
  <text class="muted status">{historyNotice}</text>
  <ActionButton
    label="Retry loading messages"
    quiet
    onclick={() =>
      void older(0).catch(() => {
        historyNotice = 'Older history could not be loaded';
      })}
  />
{/if}
{#key appearance.scale}
  <virtual-list
    class="timeline"
    alignment="bottom"
    followTail={true}
    overdraw={scaled(240)}
    estimatedItemHeight={scaled(100)}
    onvisiblerange={(event) => {
      // Image activation has one owner: the sampled native anchor below.
      // Paint reports and its conservative buffer can disagree at an edge;
      // alternating them would repeatedly discard and reload image bytes.
      void older(event.startIndex ?? 0).catch(() => {
        historyNotice = 'Older history could not be loaded';
      });
    }}
    testId={`${idPrefix}-timeline`}
    {@attach (node) => {
      listNode = node;
      const room = scope;
      let scale = appearance.scale;
      const detachZoom = beforeScaleChange((next) => {
        if (node.nativeId == null) return;
        const top = read_native((native) => native.getListScrollTop(node.nativeId!));
        if (top) zoomAnchor = { room, index: top[0] ?? 0, offset: ((top[1] ?? 0) * next) / scale };
        scale = next;
      });
      visible = { start: -1, end: -1 };
      // Native does not report every initial range or follow-tail change.
      // Like GPUix's Scroller, sample its cheap anchor read without asking
      // for bounds (bounds reads can draw a frame on other platforms).
      const timer = setInterval(() => {
        if (listNode !== node || node.nativeId == null) return;
        if (zoomAnchor?.room === room) {
          const anchor = zoomAnchor;
          zoomAnchor = null;
          read_native((native) => native.scrollToItem(node.nativeId!, anchor.index, anchor.offset));
        }
        const top = read_native((native) => native.getListScrollTop(node.nativeId!));
        if (!top) return;
        const start = top[0] ?? 0;
        // Every message row is at least 50px tall. This conservative
        // window covers visible rows without loading the whole history.
        const end = start + Math.ceil((top[2] ?? 500) / scaled(50));
        const moved = start !== visible.start;
        if (moved || end !== visible.end) visible = { start, end };
        if (moved)
          void older(start).catch(() => {
            historyNotice = 'Older history could not be loaded';
          });
      }, 200);
      return () => {
        detachZoom();
        clearInterval(timer);
        if (listNode === node) listNode = null;
      };
    }}
  >
    {#each events as item, index (item.id)}
      <LiveMessage
        {item}
        {server}
        {onOpenThread}
        dayLabel={index === 0 ||
        messageDay(events[index - 1]!.createdAt) !== messageDay(item.createdAt)
          ? messageDate(item.createdAt)
          : null}
        active={visible.start >= 0 && index >= visible.start - 2 && index <= visible.end + 2}
      />
    {/each}
  </virtual-list>
{/key}

<style>
  .timeline {
    flex-grow: 1;
    min-height: 0;
    width: 100%;
  }
  .muted {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .status {
    padding: var(--size-6) var(--size-18);
  }
  .error {
    color: var(--color-error);
    font-size: var(--text-small);
    padding: var(--size-6) var(--size-18);
    white-space: normal;
  }
</style>
