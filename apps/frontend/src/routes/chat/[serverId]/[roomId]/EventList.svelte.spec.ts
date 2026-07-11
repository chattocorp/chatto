import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import { setVirtualizerScrollOffset } from './EventListVirtualizerMock.svelte';

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
      serverInfo: { messageEditWindowSeconds: 300 }
    })
  }
}));

vi.mock('$lib/hooks/useTabResumeCallback.svelte', () => ({
  useTabResumeCallback: (callback: () => void) => resumeCallbacks.push(callback)
}));

vi.mock('$lib/hooks/useMayHaveMissedMessagesCallback.svelte', () => ({
  useMayHaveMissedMessagesCallback: () => {}
}));

describe('EventList jump completion', () => {
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

  it('starts a pending return-to-present scroll when loading finishes', async () => {
    const onJumpToPresent = vi.fn();
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
    const callsBeforeJump = Number(
      page.getByTestId('virtualizer-scroll-calls').element().textContent
    );
    await page.getByTestId('jump-to-present').click();
    expect(onJumpToPresent).toHaveBeenCalledOnce();

    await rendered.rerender({
      eventIds: ['msg-target'],
      scrollToEventId: null,
      isJumpedMode: false,
      isLoading: true,
      onJumpToPresent,
      pendingHighlightId: 'suppress-normal-auto-scroll'
    });
    await expect.element(page.getByTestId('virtualizer-scroll-calls')).not.toBeInTheDocument();

    await rendered.rerender({
      eventIds: ['msg-target'],
      scrollToEventId: null,
      isJumpedMode: false,
      isLoading: false,
      onJumpToPresent,
      pendingHighlightId: 'suppress-normal-auto-scroll'
    });
    await vi.waitFor(() =>
      expect(
        Number(page.getByTestId('virtualizer-scroll-calls').element().textContent)
      ).toBeGreaterThan(callsBeforeJump)
    );
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
          scrollToEventId: null,
          updateCounter: 0
        }
      });

      await vi.waitFor(() => expect(animationFrames.length).toBeGreaterThan(0));
      await rendered.rerender({
        eventIds: ['msg-target'],
        scrollToEventId: null,
        updateCounter: 1
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
});
