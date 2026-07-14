import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import {
  setVirtualizerForcedRenderedIndex,
  setVirtualizerScrollOffset
} from './EventListVirtualizerMock.svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';

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

vi.mock('$lib/state/userProfiles.svelte', () => ({
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

  it('preserves an expanded system group across virtual row remounts and resets it by room', async () => {
    const initialEventIds = ['join-1', 'join-2', 'join-3', 'join-4', 'join-5'];
    const rendered = render(EventListTestHarness, {
      props: {
        eventIds: initialEventIds,
        eventKind: 'join',
        scrollToEventId: null,
        updateCounter: initialEventIds.length
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
      scrollToEventId: null,
      updateCounter: extendedEventIds.length
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
      scrollToEventId: null,
      updateCounter: extendedEventIds.length
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

      await expect.element(page.getByText('Dies ist der Anfang dieser Unterhaltung.')).toBeVisible();
    } finally {
      setVirtualizerForcedRenderedIndex(null);
      await loadLocaleMessages('en-GB');
      setReactiveLocale('en-GB');
    }
  });
});
