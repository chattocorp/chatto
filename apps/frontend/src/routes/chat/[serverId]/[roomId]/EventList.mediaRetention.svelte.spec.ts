import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { render } from 'vitest-browser-svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import '../../../../app.css';

// Keep the real Virtualizer, MessageAttachments, and VideoPlayer to test callback ownership.
vi.mock('./RoomEvent.svelte', async () => {
  const { default: RoomEvent } = await import('./EventListMediaRoomEventMock.svelte');
  return { default: RoomEvent };
});

vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: () => 'server-1' }));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_userId: string, fallback: string | null) => fallback,
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

const nextFrame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

function row(id: string) {
  return document.querySelector<HTMLElement>(`[data-event-id="${id}"]`);
}

function renderTimeline(id: string, thread = false) {
  const eventIds = [...Array.from({ length: 48 }, (_, i) => `msg-${i}`), id];
  const view = render(EventListTestHarness, {
    props: {
      eventIds,
      scrollToEventId: null,
      hasReachedStart: true,
      permalinkThreadRootEventId: thread ? 'thread-root' : null
    }
  });
  view.container.style.cssText = 'display: flex; flex-direction: column; height: 200px;';
  const scroller = view.container.querySelector<HTMLElement>('[data-testid="messages-container"]')!;
  return { ...view, eventIds, scroller };
}

async function startMedia(id: string) {
  await vi.waitFor(() => expect(row(id)?.querySelector('audio, video')).toBeTruthy());
  const media = row(id)!.querySelector<HTMLMediaElement>('audio, video')!;
  // Vidstack creates its video before its provider finishes installing the source.
  await vi.waitFor(() =>
    expect(media.readyState).toBeGreaterThanOrEqual(HTMLMediaElement.HAVE_CURRENT_DATA)
  );
  media.muted = true;
  media.loop = true;
  await media.play();
  await tick();
  return media;
}

async function scrollToStart(scroller: HTMLElement, firstId = 'msg-0') {
  // Mark user intent so the timeline stops following new messages at its bottom.
  scroller.dispatchEvent(new WheelEvent('wheel', { deltaY: -1000, bubbles: true }));
  await vi.waitFor(async () => {
    scroller.scrollTop = 0;
    await nextFrame();
    expect(row(firstId)).not.toBeNull();
    expect(row('msg-46')).toBeNull();
  });
}

describe('EventList media retention', () => {
  beforeEach(async () => {
    createTestServerScope({
      store: { realtimeSync: { isRecoveringSnapshot: false } },
      serverInfo: { messageEditWindowSeconds: 300 },
      api: { refreshAssetUrls: async () => new Map() }
    });
    // Audio playback needs a real user gesture, even when it is muted.
    await userEvent.click(document.body);
  });

  afterEach(() => {
    document.getSelection()?.removeAllRanges();
  });

  it.each([
    ['audio', false],
    ['video', false],
    ['processed', false],
    ['audio', true],
    ['video', true],
    ['processed', true]
  ] as const)('keeps playing %s mounted (thread: %s)', async (kind, thread) => {
    const id = `${kind}-playing`;
    const { scroller } = renderTimeline(id, thread);
    const media = await startMedia(id);
    await scrollToStart(scroller);

    expect(row(id)?.querySelector('audio, video')).toBe(media);
    expect(media.getBoundingClientRect().top).toBeGreaterThan(
      scroller.getBoundingClientRect().bottom
    );
    expect(media.paused).toBe(false);
    const position = media.currentTime;
    await vi.waitFor(() => expect(media.currentTime).not.toBe(position));
    media.pause();
    await vi.waitFor(async () => {
      scroller.scrollTop = 0;
      await nextFrame();
      expect(row(id)).toBeNull();
    });
  });

  it.each([
    ['audio', 'ended'],
    ['audio', 'error'],
    ['audio', 'emptied'],
    ['video', 'ended'],
    ['video', 'error'],
    ['video', 'emptied'],
    ['processed', 'ended'],
    ['processed', 'error'],
    ['processed', 'emptied']
  ])('releases offscreen %s on %s', async (kind, type) => {
    const id = `${kind}-playing`;
    const { scroller } = renderTimeline(id);
    const media = await startMedia(id);
    await scrollToStart(scroller);
    (media.closest('media-player') ?? media).dispatchEvent(new Event(type));
    await vi.waitFor(() => expect(row(id)).toBeNull());
  });

  it.each(['video', 'processed'])(
    'keeps offscreen %s mounted after pause until picture-in-picture ends',
    async (kind) => {
      const id = `${kind}-playing`;
      const { scroller } = renderTimeline(id);
      const media = await startMedia(id);
      // Headless browsers cannot open a real picture-in-picture window.
      media.dispatchEvent(new Event('enterpictureinpicture'));
      await scrollToStart(scroller);
      media.pause();
      await vi.waitFor(() => expect(media.paused).toBe(true));
      scroller.scrollTop = 0;
      await nextFrame();
      await nextFrame();
      expect(row(id)?.querySelector('video')).toBe(media);
      media.dispatchEvent(new Event('leavepictureinpicture'));
      await vi.waitFor(() => expect(row(id)).toBeNull());
    }
  );

  it('retains a row until all of its playing attachments pause', async () => {
    const id = 'multiple-playing';
    const { scroller } = renderTimeline(id);
    const first = await startMedia(id);
    const second = row(id)!.querySelectorAll('audio')[1];
    second.muted = true;
    await second.play();
    await tick();
    await scrollToStart(scroller);
    first.pause();
    await tick();
    expect(row(id)?.querySelectorAll('audio')[1]).toBe(second);
    second.pause();
    await vi.waitFor(() => expect(row(id)).toBeNull());
  });

  it('keeps buffering and seeking media mounted', async () => {
    const id = 'audio-playing';
    const { scroller } = renderTimeline(id);
    const media = await startMedia(id);
    media.dispatchEvent(new Event('waiting'));
    media.dispatchEvent(new Event('seeking'));
    await scrollToStart(scroller);
    expect(row(id)?.querySelector('audio')).toBe(media);
  });

  it('does not retain autoplaying converted GIFs', async () => {
    const id = 'gif-loop';
    const { scroller } = renderTimeline(id);
    await vi.waitFor(() => expect(row(id)?.querySelector('video')?.paused).toBe(false));
    await scrollToStart(scroller);
    expect(row(id)).toBeNull();
  });

  it.each(['audio', 'video', 'processed'])(
    'releases a row when its playing %s is removed',
    async (kind) => {
      const id = `${kind}-playing`;
      const { scroller } = renderTimeline(id);
      const media = await startMedia(id);
      await scrollToStart(scroller);
      row(id)!.querySelector<HTMLButtonElement>('[data-testid="remove-attachments"]')!.click();
      await vi.waitFor(() => expect(row(id)).toBeNull());
      expect(media.isConnected).toBe(false);
    }
  );

  it('does not retain a deleted message when its key is reused', async () => {
    const id = 'audio-playing';
    const { scroller, eventIds, rerender } = renderTimeline(id);
    const media = await startMedia(id);
    await scrollToStart(scroller);
    await rerender({ eventIds: eventIds.slice(0, -1) });
    await vi.waitFor(() => expect(media.isConnected).toBe(false));
    await rerender({ eventIds });
    await nextFrame();
    expect(row(id)).toBeNull();
  });

  it.each(['audio', 'video', 'processed'])(
    'combines selection and %s playback after older rows are prepended',
    async (kind) => {
      const id = `${kind}-playing`;
      const { scroller, eventIds, rerender } = renderTimeline(id);
      const media = await startMedia(id);
      // A tall media row can leave the preceding text outside virtua's mounted range.
      scroller.dispatchEvent(new WheelEvent('wheel', { deltaY: -200, bubbles: true }));
      await vi.waitFor(async () => {
        const offset = row(id)!.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
        scroller.scrollTop += offset - scroller.clientHeight / 2;
        await nextFrame();
        expect(row('msg-47')).not.toBeNull();
      });
      const anchor = row('msg-47')!.querySelector('span')!.firstChild!;
      const selection = document.getSelection()!;
      selection.setBaseAndExtent(anchor, 0, anchor, 0);
      await nextFrame();
      await scrollToStart(scroller);
      const focus = row('msg-0')!.querySelector('span')!.firstChild!;
      selection.setBaseAndExtent(anchor, anchor.textContent!.length, focus, 0);
      await tick();
      await vi.waitFor(() => expect(row('msg-24')).not.toBeNull());
      await rerender({
        eventIds: [...Array.from({ length: 8 }, (_, i) => `older-${i}`), ...eventIds]
      });
      expect(row(id)?.querySelector('audio, video')).toBe(media);
      expect(document.contains(anchor)).toBe(true);
      selection.removeAllRanges();
      await nextFrame();
      await scrollToStart(scroller, 'older-0');
      expect(row(id)?.querySelector('audio, video')).toBe(media);
    }
  );

  it('removes retained media when the timeline unmounts', async () => {
    const id = 'audio-playing';
    const { scroller, unmount } = renderTimeline(id);
    const media = await startMedia(id);
    await scrollToStart(scroller);
    await unmount();
    expect(media.isConnected).toBe(false);
  });
});
