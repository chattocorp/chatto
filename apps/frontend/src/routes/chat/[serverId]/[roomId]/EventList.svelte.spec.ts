import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import {
  setVirtualizerForcedRenderedIndex,
  setVirtualizerScrollOffset
} from './EventListVirtualizerMock.svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import type { JumpToMessageState } from '$lib/state/room';

const resumeCallbacks = vi.hoisted(() => [] as Array<() => void>);

vi.mock('virtua/svelte', async () => {
  const { default: Virtualizer } = await import('./EventListVirtualizerMock.svelte');
  return { Virtualizer };
});

vi.mock('./RoomEvent.svelte', async () => {
  const { default: RoomEvent } = await import('./EventListRoomEventMock.svelte');
  return { default: RoomEvent };
});

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server-1'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      currentUser: { user: { id: 'test-user' } },
      realtimeSync: { isRecoveringSnapshot: false },
      serverInfo: { messageEditWindowSeconds: 300 }
    })
  }
}));

vi.mock('$lib/state/server/scope.svelte', async () => {
  const { serverRegistry } = await import('$lib/state/server/registry.svelte');
  return {
    useServerScope: () => ({
      serverId: 'server-1',
      connection: {},
      get store() {
        return serverRegistry.getStore('server-1');
      }
    })
  };
});

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: unknown) => fallback
  })
}));

vi.mock('$lib/hooks/useTabResumeCallback.svelte', () => ({
  useTabResumeCallback: (callback: () => void) => resumeCallbacks.push(callback)
}));

describe('EventList jump completion', () => {
  it('stops pending bottom scrolling before reading an unmounted room', async () => {
    const frames: FrameRequestCallback[] = [];
    const requestFrame = vi
      .spyOn(window, 'requestAnimationFrame')
      .mockImplementation((callback) => {
        frames.push(callback);
        return frames.length;
      });
    const onStoreRead = vi.fn();
    const view = render(EventListTestHarness, {
      props: { eventIds: ['message'], scrollToEventId: null, onStoreRead }
    });
    try {
      await tick();
      expect(frames.length).toBeGreaterThan(0);
      view.unmount();
      onStoreRead.mockClear();
      for (const frame of frames.splice(0)) frame(performance.now());
      await tick();
      expect(onStoreRead).not.toHaveBeenCalled();
    } finally {
      requestFrame.mockRestore();
    }
  });
  it('releases interrupted forward pagination when the snapshot viewport is restored', async () => {
    let jumpState!: JumpToMessageState;
    const rendered = render(EventListTestHarness, {
      props: {
        eventIds: [],
        scrollToEventId: null,
        isLoading: true,
        recoveryViewport: { eventId: 'msg-anchor', offset: 17, hasNewer: true },
        onComposerReady: (context) => {
          jumpState = context.jumpState;
          jumpState.isJumpedMode = true;
          // Snapshot invalidation prevents the old request's finally block
          // from changing pagination state for the replacement window.
          jumpState.isLoadingNewer = true;
        }
      }
    });
    await rendered.rerender({ eventIds: ['msg-anchor'], isLoading: false });
    await expect.element(page.getByTestId('virtualizer-scroll-offset')).toHaveTextContent('17');
    expect(jumpState.isLoadingNewer).toBe(false);
    expect(jumpState.isJumpedMode).toBe(true);
    expect(jumpState.hasReachedEnd).toBe(false);
  });
  it('releases the saved position when recovery produces an empty timeline', async () => {
    render(EventListTestHarness, {
      props: {
        eventIds: [],
        scrollToEventId: null,
        recoveryViewport: { eventId: 'removed', offset: 17 }
      }
    });
    await vi.waitFor(() =>
      expect(page.getByTestId('recovery-anchor').element().textContent).toBe('')
    );
  });
  it('restores the saved event and pixel offset after a cleared timeline loads', async () => {
    const rendered = render(EventListTestHarness, {
      props: {
        eventIds: [],
        scrollToEventId: null,
        isLoading: true,
        recoveryViewport: { eventId: 'msg-anchor', offset: 17, hasNewer: true }
      }
    });
    await rendered.rerender({
      eventIds: ['msg-before', 'msg-anchor', 'msg-after'],
      scrollToEventId: null,
      isLoading: false,
      recoveryViewport: { eventId: 'msg-anchor', offset: 17, hasNewer: true }
    });
    await expect.element(page.getByText('msg-anchor', { exact: true })).toBeVisible();
    await expect
      .element(page.getByTestId('virtualizer-scroll-alignment'))
      .toHaveTextContent('start');
    await expect.element(page.getByTestId('virtualizer-scroll-offset')).toHaveTextContent('17');
  });
  it('signals completion after highlighting a rendered target', async () => {
    const onComplete = vi.fn();
    render(EventListTestHarness, {
      props: {
        eventIds: ['msg-target'],
        scrollToEventId: 'msg-target',
        onComplete
      }
    });

    await expect.element(page.getByText('msg-target', { exact: true })).toBeInTheDocument();
    await expect.element(page.getByTestId('virtualizer-scroll-index')).not.toHaveTextContent('');
    await vi.waitFor(() => expect(onComplete).toHaveBeenCalledExactlyOnceWith(true));
  });

  it('signals completion after bounded retries when the target is not rendered', async () => {
    const onComplete = vi.fn();
    render(EventListTestHarness, {
      props: {
        eventIds: ['msg-other'],
        scrollToEventId: 'msg-target',
        onComplete
      }
    });

    await vi.waitFor(() => expect(onComplete).toHaveBeenCalledExactlyOnceWith(false), {
      timeout: 2_000
    });
  });

  it('cancels completion for a superseded scroll target', async () => {
    const onComplete = vi.fn();
    const rendered = render(EventListTestHarness, {
      props: {
        eventIds: ['msg-new'],
        scrollToEventId: 'msg-old',
        onComplete
      }
    });

    await rendered.rerender({
      eventIds: ['msg-new'],
      scrollToEventId: 'msg-new',
      onComplete
    });

    await expect.element(page.getByText('msg-new', { exact: true })).toBeInTheDocument();
    await vi.waitFor(() => expect(onComplete).toHaveBeenCalledExactlyOnceWith(true));
  });

  it('cancels a pending scroll attempt when unmounted', async () => {
    const animationFrames: FrameRequestCallback[] = [];
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        animationFrames.push(callback);
        return animationFrames.length;
      })
    );
    const onComplete = vi.fn();
    try {
      const rendered = render(EventListTestHarness, {
        props: {
          eventIds: ['msg-other'],
          scrollToEventId: 'msg-never-mounted',
          onComplete
        }
      });

      await vi.waitFor(() => expect(animationFrames.length).toBeGreaterThan(0));
      rendered.unmount();
      for (let index = 0; index < 100 && animationFrames[index]; index++) {
        animationFrames[index](index * 16);
      }

      expect(onComplete).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('scrolls to present after the latest window finishes loading', async () => {
    let finishLoading: ((loaded: boolean) => void) | undefined;
    const latestLoaded = new Promise<boolean>((resolve) => {
      finishLoading = resolve;
    });
    const onJumpToPresent = vi.fn(() => latestLoaded);
    const rendered = render(EventListTestHarness, {
      props: {
        eventIds: ['msg-target'],
        scrollToEventId: 'msg-target',
        isJumpedMode: true,
        onJumpToPresent,
        pendingHighlightId: 'suppress-normal-auto-scroll'
      }
    });

    await expect.element(page.getByTestId('jump-to-present')).toBeVisible();
    expect(page.getByTestId('jump-to-present').element().classList).toContain('z-40');
    await expect
      .element(page.getByTestId('virtualizer-scroll-alignment'))
      .toHaveTextContent('center');
    (page.getByTestId('jump-to-present').element() as HTMLButtonElement).click();
    expect(onJumpToPresent).toHaveBeenCalledOnce();
    await expect
      .element(page.getByTestId('virtualizer-scroll-alignment'))
      .toHaveTextContent('center');

    finishLoading?.(true);
    await rendered.rerender({
      eventIds: ['msg-target'],
      scrollToEventId: null,
      isJumpedMode: false,
      onJumpToPresent,
      pendingHighlightId: 'suppress-normal-auto-scroll'
    });
    await expect.element(page.getByTestId('virtualizer-scroll-alignment')).toHaveTextContent('end');
  });

  it('completes initialization when a bottom scroll supersedes the initial request', async () => {
    const animationFrames: FrameRequestCallback[] = [];
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        animationFrames.push(callback);
        return animationFrames.length;
      })
    );
    try {
      const rendered = render(EventListTestHarness, {
        props: {
          eventIds: ['msg-target'],
          scrollToEventId: null
        }
      });

      await vi.waitFor(() => expect(animationFrames.length).toBeGreaterThan(0));
      // A newly arrived message starts a second bottom scroll.
      await rendered.rerender({
        eventIds: ['msg-target', 'msg-next'],
        scrollToEventId: null
      });

      for (let frame = 0; frame < 50; frame++) {
        await vi.waitFor(() => expect(animationFrames.length).toBeGreaterThan(0));
        animationFrames.shift()?.(frame * 16);
        if (Number(page.getByTestId('virtualizer-scroll-calls').element().textContent) >= 7) {
          break;
        }
      }
      await vi.waitFor(() =>
        expect(
          Number(page.getByTestId('virtualizer-scroll-calls').element().textContent)
        ).toBeGreaterThanOrEqual(7)
      );
      await Promise.resolve();

      const resume = resumeCallbacks.at(-1);
      expect(resume).toBeDefined();
      setVirtualizerScrollOffset(400);
      resume?.();
      await expect.element(page.getByTestId('jump-to-present')).toBeVisible();
    } finally {
      setVirtualizerScrollOffset(700);
      vi.unstubAllGlobals();
    }
  });

  it('preserves an expanded system group across virtual row remounts and resets it by room', async () => {
    const initialEventIds = ['join-1', 'join-2', 'join-3', 'join-4', 'join-5'];
    const rendered = render(EventListTestHarness, {
      props: {
        eventIds: initialEventIds,
        eventKind: 'join',
        scrollToEventId: null
      }
    });

    await page.getByRole('button', { name: '2 others' }).click();
    await expect.element(page.getByRole('button', { name: 'show less' })).toBeVisible();

    // Extending the group changes its virtual-item key, forcing the mock
    // virtualizer to remount the row just like forward pagination can.
    const extendedEventIds = [...initialEventIds, 'join-6'];
    await rendered.rerender({
      eventIds: extendedEventIds,
      eventKind: 'join',
      scrollToEventId: null
    });
    await expect
      .element(page.getByTestId('virtualizer-rendered-key'))
      .toHaveAttribute('data-rendered-key', 'system-group-join-6');
    await expect.element(page.getByRole('button', { name: 'show less' })).toBeVisible();

    await page.getByRole('button', { name: 'show less' }).click();
    await expect.element(page.getByRole('button', { name: '3 others' })).toBeVisible();

    await page.getByRole('button', { name: '3 others' }).click();
    await rendered.rerender({
      eventIds: extendedEventIds,
      roomId: 'room-2',
      eventKind: 'join',
      scrollToEventId: null
    });
    await expect.element(page.getByRole('button', { name: '3 others' })).toBeVisible();
  });
});

describe('EventList localisation', () => {
  it('localises the beginning-of-conversation marker', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    setVirtualizerForcedRenderedIndex(0);

    try {
      render(EventListTestHarness, {
        props: {
          eventIds: ['msg-first'],
          scrollToEventId: null,
          hasReachedStart: true
        }
      });

      await expect
        .element(page.getByText('Dies ist der Anfang dieser Unterhaltung.'))
        .toBeVisible();
    } finally {
      setVirtualizerForcedRenderedIndex(null);
      await loadLocaleMessages('en-GB');
      setReactiveLocale('en-GB');
    }
  });
});

describe('EventList unread entry landing', () => {
  const eventIds = ['msg-1', 'msg-2', 'msg-3'];

  async function nextFrames(count = 3) {
    for (let frame = 0; frame < count; frame++) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    }
  }

  it('lands on the unread separator when the marker resolves after entry', async () => {
    setVirtualizerScrollOffset(400);
    try {
      const rendered = render(EventListTestHarness, {
        props: { eventIds, scrollToEventId: null }
      });
      await expect
        .element(page.getByTestId('virtualizer-scroll-alignment'))
        .toHaveTextContent('end');

      await rendered.rerender({
        eventIds,
        scrollToEventId: null,
        unreadAfterEventId: 'msg-2'
      });

      await expect
        .element(page.getByTestId('virtualizer-rendered-key'))
        .toHaveAttribute('data-rendered-key', 'unread-separator-msg-2');
      await expect
        .element(page.getByTestId('virtualizer-scroll-alignment'))
        .toHaveTextContent('start');
      await expect.element(page.getByTestId('jump-to-present')).toBeVisible();
    } finally {
      setVirtualizerScrollOffset(700);
    }
  });

  it('lets the jump button cancel a running landing', async () => {
    const frames: FrameRequestCallback[] = [];
    const requestFrame = vi
      .spyOn(window, 'requestAnimationFrame')
      .mockImplementation((callback) => {
        frames.push(callback);
        return frames.length;
      });
    setVirtualizerScrollOffset(400);
    try {
      render(EventListTestHarness, {
        props: {
          eventIds,
          scrollToEventId: null,
          unreadAfterEventId: 'msg-2'
        }
      });
      await expect.element(page.getByTestId('jump-to-present')).toBeVisible();

      (page.getByTestId('jump-to-present').element() as HTMLButtonElement).click();
      for (let frame = 0; frame < 60 && frames.length > 0; frame++) {
        frames.shift()?.(frame * 16);
        await tick();
        await Promise.resolve();
      }
      // Real frames let the button's fade-out transition finish.
      requestFrame.mockRestore();

      await expect.element(page.getByTestId('jump-to-present')).not.toBeInTheDocument();
      expect(page.getByTestId('virtualizer-scroll-alignment').element().textContent).toBe('end');
    } finally {
      requestFrame.mockRestore();
      setVirtualizerScrollOffset(700);
    }
  });

  it('skips the landing when the entry targets a specific message', async () => {
    const rendered = render(EventListTestHarness, {
      props: {
        eventIds,
        scrollToEventId: null,
        unreadAfterEventId: 'msg-2',
        pendingHighlightId: 'msg-3'
      }
    });
    await nextFrames();

    // The highlight can clear without a jump, for example when the target
    // is missing. The entry still does not move to the separator.
    await rendered.rerender({
      eventIds,
      scrollToEventId: null,
      unreadAfterEventId: 'msg-2',
      pendingHighlightId: null
    });
    await expect.element(page.getByTestId('virtualizer-scroll-alignment')).toHaveTextContent('end');
    await nextFrames();

    expect(page.getByTestId('virtualizer-scroll-alignment').element().textContent).toBe('end');
  });

  it('lands again when a thread timeline switches to another thread', async () => {
    const threadProps = (threadId: string, marker: string | null) => ({
      eventIds,
      permalinkThreadRootEventId: threadId,
      scrollToEventId: null,
      unreadAfterEventId: marker
    });
    const rendered = render(EventListTestHarness, { props: threadProps('thread-1', 'msg-2') });
    await expect
      .element(page.getByTestId('virtualizer-rendered-key'))
      .toHaveAttribute('data-rendered-key', 'unread-separator-msg-2');

    await rendered.rerender(threadProps('thread-2', null));
    await rendered.rerender({ ...threadProps('thread-2', null), eventIds: [...eventIds, 'msg-4'] });
    await expect.element(page.getByTestId('virtualizer-scroll-alignment')).toHaveTextContent('end');

    await rendered.rerender({
      ...threadProps('thread-2', 'msg-3'),
      eventIds: [...eventIds, 'msg-4']
    });
    await expect
      .element(page.getByTestId('virtualizer-scroll-alignment'))
      .toHaveTextContent('start');
    await expect
      .element(page.getByTestId('virtualizer-rendered-key'))
      .toHaveAttribute('data-rendered-key', 'unread-separator-msg-3');
  });

  it('keeps the viewport when the user scrolls before the marker resolves', async () => {
    const rendered = render(EventListTestHarness, {
      props: { eventIds, scrollToEventId: null }
    });
    await expect.element(page.getByTestId('virtualizer-scroll-alignment')).toHaveTextContent('end');

    page
      .getByTestId('messages-container')
      .element()
      .dispatchEvent(new WheelEvent('wheel', { deltaY: -100, bubbles: true }));
    await rendered.rerender({
      eventIds,
      scrollToEventId: null,
      unreadAfterEventId: 'msg-2'
    });
    await nextFrames();

    expect(page.getByTestId('virtualizer-scroll-alignment').element().textContent).toBe('end');
  });
});
