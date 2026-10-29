import type { DMData } from '$lib/hooks/useRoomData.svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { tick } from 'svelte';
import { q } from '$lib/test-utils';
import { TimelineEventKind, type TimelineEventView } from '@chatto/client/timeline/timelineEvents';
import { threadPaneWidth } from '$lib/state/threadPaneWidth.svelte';
import { THREAD_PANE_MAX_WIDTH } from '$lib/storage/threadPaneWidth';
import { getToasts, toast } from '$lib/ui/toast';
import ThreadPane from './ThreadPane.svelte';
import ConversationPane from './ConversationPane.svelte';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import { ThreadPaneTestStore } from './ThreadPaneTestStore.svelte';
import { RealtimeProjectionUpdate } from '@chatto/client/realtime/eventBus';
import { MessagePostedEvent } from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEvent as PublicRealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { signal } from '@chatto/client/reactivity';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { RealtimeResourceUpdate } from '@chatto/client/api/realtimeResources';
import { ListNotificationOccurrencesResponse } from '@chatto/api-types/api/v1/notifications_pb';
import {
  NotificationAttentionLevel,
  NotificationSignalKind,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';

const { mocks } = vi.hoisted(() => {
  return {
    mocks: {
      dmData: null as DMData | null,
      markThreadAsRead: vi.fn(),
      registerReadView: vi.fn(() => vi.fn()),
      reconcileThreadRead: vi.fn(),
      followThread: vi.fn(),
      unfollowThread: vi.fn(),
      setThread: vi.fn(),
      disposeMessagesStore: vi.fn(),
      ingestEvent: vi.fn(),
      refreshCurrentWindow: vi.fn(),
      restoreLatestWindow: vi.fn(),
      clearAnchor: vi.fn(),
      setThreadRootFollowState: vi.fn(),
      loadMore: vi.fn(),
      removeTypingUser: vi.fn(),
      sendTypingIndicator: vi.fn(),
      resetTypingDebounce: vi.fn(),
      jumpToMessage: vi.fn(),
      storeJumpToMessage: vi.fn(),
      resetJumpState: vi.fn(),
      startReply: vi.fn(),
      cancelReply: vi.fn(),
      requestInsertQuote: vi.fn(),
      cancelEdit: vi.fn(),
      editingEventId: null as string | null,
      markOccurrenceRead: vi.fn(),
      jumpState: null as {
        scrollToEventId: string | null;
        jumpToMessage: (eventId: string) => Promise<boolean>;
      } | null,
      onClose: vi.fn(),
      clearUnreadMarker: vi.fn(),
      markArrivalWhileAway: vi.fn(),
      projectionEventHandler: null as ((event: RealtimeProjectionUpdate) => void) | null,
      unreadMarkerEventId: null as string | null,
      canMarkThreadAsRead: null as (() => boolean) | null,
      realUnreadLifecycle: false,
      appState: {
        isPresent: true,
        isVisible: true,
        foregroundRevision: 0,
        onlineRevision: 0
      },
      threadStore: null as ThreadPaneTestStore | null
    }
  };
});

let server: TestServerScope;
const reactionOccurrences = signal<NotificationOccurrenceItem[]>([]);
const present = signal(true);
const liveParticipantProfiles = signal<Record<string, { displayName: string; login: string }>>({});

/** The event bus delivers resources after the store has applied their rows. */
function notifyReactionUpdate() {
  mocks.projectionEventHandler?.(
    new RealtimeProjectionUpdate({
      resource: new RealtimeResourceUpdate({
        resource: { case: 'notifications', value: new ListNotificationOccurrencesResponse() }
      })
    })
  );
}

function rootReaction(
  id: string,
  overrides: Partial<NotificationOccurrenceItem> = {}
): NotificationOccurrenceItem {
  return {
    id,
    createdAt: '2026-10-16T12:00:00Z',
    actor: null,
    signalKind: NotificationSignalKind.REACTION,
    targetSupported: true,
    room: { id: 'room-1', name: 'General' },
    eventId: 'thread-root',
    threadRootId: null,
    attentionLevel: NotificationAttentionLevel.AMBIENT,
    unread: true,
    reactionEmoji: 'heart',
    ...overrides
  };
}

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: { getStore: () => server.scope.store }
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_id: string, fallback: string | null) => fallback,
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (id: string, fallback: string) =>
    liveParticipantProfiles.get()[id]?.displayName ?? fallback,
  getLiveLogin: (id: string, fallback: string) =>
    liveParticipantProfiles.get()[id]?.login ?? fallback,
  getLiveAvatarUrl: (_id: string, fallback: string | null) => fallback,
  getLiveCustomStatus: () => null
}));

vi.mock('@chatto/client/api/readState', () => ({
  createReadStateAPI: () => ({
    markThreadAsRead: mocks.markThreadAsRead
  })
}));

vi.mock('@chatto/client/api/threads', () => ({
  createThreadAPI: () => ({
    followThread: mocks.followThread,
    unfollowThread: mocks.unfollowThread
  })
}));

vi.mock('$lib/hooks', async () => {
  const { useUnreadMarker } = await import('$lib/hooks/useUnreadMarker.svelte');
  return {
    useRoomData: () => ({ dmData: mocks.dmData }),
    useProjectionEvent: (handler: (event: RealtimeProjectionUpdate) => void) => {
      mocks.projectionEventHandler = handler;
    },
    // ConversationPane uses this only for the room timeline.
    useRoomUnread: vi.fn(),
    useUnreadMarker: (
      getTargetId: () => string,
      options: Parameters<typeof useUnreadMarker>[1]
    ) => {
      mocks.canMarkThreadAsRead = options.canMarkAsRead ?? null;
      if (mocks.realUnreadLifecycle) return useUnreadMarker(getTargetId, options);
      void options.markAsRead(getTargetId(), undefined, new AbortController().signal);
      return {
        unreadMarkerEventId: mocks.unreadMarkerEventId,
        markAsRead: (targetId: string, upToEventId?: string) =>
          options.markAsRead(targetId, upToEventId, new AbortController().signal),
        clearUnreadMarker: mocks.clearUnreadMarker,
        markArrivalWhileAway: mocks.markArrivalWhileAway
      };
    },
    createTypingIndicator: () => ({
      userIds: [],
      removeTypingUser: mocks.removeTypingUser,
      sendTypingIndicator: mocks.sendTypingIndicator,
      resetDebounce: mocks.resetTypingDebounce
    })
  };
});

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server-1'
}));

vi.mock('$lib/state/globals.svelte', () => ({
  sidebarNav: vi.fn(),
  quickSwitcher: vi.fn(),
  appState: mocks.appState
}));

vi.mock('$lib/state/room', () => ({
  getRoomMembers: () => [],
  getComposerContext: () => ({ jumpState: mocks.jumpState }),
  createComposerContext: () => ({
    editState: {
      get eventId() {
        return mocks.editingEventId;
      },
      cancelEdit: mocks.cancelEdit
    },
    replyState: {
      messageEventId: null,
      actorDisplayName: '',
      excerpt: '',
      startReply: mocks.startReply,
      cancelReply: mocks.cancelReply
    },
    quoteInsertionState: {
      requestInsertQuote: mocks.requestInsertQuote
    },
    jumpState: (() => {
      // Route jumps through the pane's registered handler, as the real state does.
      let handler: ((eventId: string) => Promise<boolean>) | null = null;
      const scrollTarget = signal<string | null>(null);
      const jumpState = {
        get scrollToEventId() {
          return scrollTarget.get();
        },
        set scrollToEventId(eventId: string | null) {
          scrollTarget.set(eventId);
        },
        setJumpHandler: (fn: (eventId: string) => Promise<boolean>) => {
          handler = fn;
        },
        jumpToMessage: (eventId: string) => {
          mocks.jumpToMessage(eventId);
          return handler ? handler(eventId) : Promise.resolve(false);
        },
        // Scroll to a message that the store shows, as the real state does.
        show: async (
          store: { jumpToMessage: (eventId: string) => Promise<{ status: string } | undefined> },
          eventId: string
        ) => {
          const result = await store.jumpToMessage(eventId);
          const shown = result?.status === 'shown' || result?.status === 'loaded';
          jumpState.scrollToEventId = shown ? eventId : null;
          return shown;
        },
        reset: mocks.resetJumpState
      };
      mocks.jumpState = jumpState;
      return jumpState;
    })()
  }),
  MessagesStore: class {
    threadEvents = [];
    isInitialLoading = false;
    isLoadingMore = false;
    hasReachedStart = true;
    setThread = mocks.setThread;
    dispose = mocks.disposeMessagesStore;
    ingestEvent = mocks.ingestEvent;
    refreshCurrentWindow = mocks.refreshCurrentWindow;
    setThreadRootFollowState = mocks.setThreadRootFollowState;
    loadMore = mocks.loadMore;
  }
}));

vi.mock('./EventList.svelte', async () => {
  const { default: EventListContractMock } = await import('./EventListContractMock.svelte');
  return { default: EventListContractMock };
});

vi.mock('$lib/components/composer/MessageComposer.svelte', async () => {
  const { default: ComposerMock } = await import('./ThreadComposerPermissionMock.svelte');
  return { default: ComposerMock };
});

function threadMessage(id: string, deletedAt: string | null = null) {
  return {
    id,
    createdAt: '2026-07-04T12:00:00Z',
    actorId: 'test-user',
    actor: null,
    event: { kind: TimelineEventKind.MessagePosted, deletedAt }
  } as never;
}

function highlight(eventId: string, notificationId: string | null = null) {
  return { roomId: 'room-1', threadRootEventId: 'thread-root', eventId, notificationId };
}

/** Stores of threads other than the default one, by thread root event ID. */
const threadStores = new Map<string, ThreadPaneTestStore>();

const threadProps = {
  roomId: 'room-1',
  roomName: 'General',
  threadRootEventId: 'thread-root',
  onClose: () => {}
};

/** A root summary can include participants whose replies are outside the loaded window. */
function participantRoot(ids: string[], count = ids.length): TimelineEventView {
  return {
    id: 'thread-root',
    createdAt: '2026-10-28T12:00:00Z',
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room-1',
      body: 'Thread root',
      attachments: [],
      reactions: [],
      replyCount: count,
      threadParticipantCount: count,
      threadParticipants: ids.map((id) => ({
        id,
        login: id,
        displayName: id,
        presenceStatus: PresenceStatus.OFFLINE
      }))
    }
  };
}

describe('ThreadPane', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    liveParticipantProfiles.set({});
    mocks.realUnreadLifecycle = false;
    reactionOccurrences.set([]);
    Object.defineProperty(mocks.appState, 'isPresent', {
      configurable: true,
      get: () => present.get(),
      set: (value: boolean) => present.set(value)
    });
    mocks.dmData = null;
    localStorage.clear();
    threadPaneWidth.reset();
    mocks.threadStore = new ThreadPaneTestStore();
    threadStores.clear();
    threadStores.set('thread-2', new ThreadPaneTestStore());
    // Like the real store, scroll only to a message that the thread window contains.
    mocks.storeJumpToMessage.mockImplementation(async (eventId: string) =>
      mocks.threadStore!.threadEvents.some((event) => event.id === eventId)
        ? { status: 'shown' }
        : { status: 'missing' }
    );
    server = createTestServerScope({
      viewer: { id: 'test-user', login: 'testuser' },
      store: {
        pendingHighlights: { has: () => false },
        realtimeSync: { isRecoveringSnapshot: false },
        readViews: { register: mocks.registerReadView },
        notifications: {
          markOccurrenceRead: mocks.markOccurrenceRead,
          get occurrences() {
            return reactionOccurrences.get();
          }
        },
        reconcileThreadRead: mocks.reconcileThreadRead,
        rooms: {
          // Like the registry, return one store for each thread.
          thread: (_roomId: string, threadRootEventId: string) =>
            Object.assign(threadStores.get(threadRootEventId) ?? mocks.threadStore!, {
              isLoadingMore: false,
              hasReachedStart: true,
              setThread: mocks.setThread,
              dispose: mocks.disposeMessagesStore,
              ingestEvent: mocks.ingestEvent,
              refreshCurrentWindow: mocks.refreshCurrentWindow,
              jumpToMessage: mocks.storeJumpToMessage,
              restoreLatestWindow: mocks.restoreLatestWindow,
              clearAnchor: mocks.clearAnchor,
              setThreadRootFollowState: mocks.setThreadRootFollowState,
              loadMore: mocks.loadMore
            })
        }
      }
    });
    mocks.appState.isPresent = true;
    mocks.markArrivalWhileAway.mockClear();
    mocks.projectionEventHandler = null;
    mocks.unreadMarkerEventId = null;
    mocks.editingEventId = null;
    toast.clear();
    mocks.markOccurrenceRead.mockResolvedValue(undefined);
    mocks.markThreadAsRead.mockResolvedValue({
      previousLastReadAt: null,
      lastReadAt: '2026-07-04T13:00:00Z'
    });
    mocks.followThread.mockResolvedValue({
      following: true,
      state: { roomId: 'room-1', threadRootEventId: 'thread-root', following: true }
    });
    mocks.unfollowThread.mockResolvedValue({
      following: false,
      state: { roomId: 'room-1', threadRootEventId: 'thread-root', following: false }
    });
  });

  it('shows participant summary updates and opens profiles from the header', async () => {
    const onOpenProfile = vi.fn();
    const rendered = render(ThreadPane, { props: { ...threadProps, onOpenProfile } });
    expect(
      rendered.container.querySelector('[data-testid="thread-header-participants"]')
    ).toBeNull();

    mocks.threadStore!.threadEvents = [participantRoot(['Alice', 'Bob', 'Carol', 'Dave'], 12)];
    const group = rendered.getByRole('group', { name: 'Thread participants (12)' });
    await expect.element(group).toBeVisible();
    await expect.element(group.getByText('+8')).toBeVisible();
    await expect.element(group.getByRole('button', { name: 'Alice', exact: true })).toBeVisible();
    await expect.element(group.getByRole('button', { name: 'Carol', exact: true })).toBeVisible();
    await expect.element(group.getByRole('button', { name: 'Dave', exact: true })).toBeVisible();
    const bobAvatar = group.getByRole('button', { name: 'Bob', exact: true });
    await expect.element(bobAvatar).not.toHaveAttribute('title');
    await bobAvatar.hover();
    await expect.element(page.getByRole('tooltip').filter({ hasText: 'Bob' })).toBeVisible();
    await bobAvatar.click();
    await expect
      .element(page.getByRole('tooltip').filter({ hasText: 'Bob' }))
      .not.toBeInTheDocument();
    const menu = page.getByRole('dialog', { name: 'User profile' });
    await expect.element(menu).toBeVisible();
    expect(onOpenProfile).not.toHaveBeenCalled();
    await menu.getByRole('button', { name: 'View profile' }).click();
    expect(onOpenProfile).toHaveBeenCalledWith('Bob');

    mocks.threadStore!.threadEvents = [participantRoot(['Carol'])];
    await expect
      .element(rendered.getByRole('group', { name: 'Thread participants (1)' }))
      .toBeVisible();
    await expect
      .element(rendered.getByRole('button', { name: 'Alice', exact: true }))
      .not.toBeInTheDocument();
    await rendered.rerender({ ...threadProps, onOpenProfile, threadRootEventId: 'thread-2' });
    expect(
      rendered.container.querySelector('[data-testid="thread-header-participants"]')
    ).toBeNull();
  });

  it('updates participant labels and an open hint when the live profile changes', async () => {
    mocks.threadStore!.threadEvents = [participantRoot(['Alice'])];
    const rendered = render(ThreadPane, { props: threadProps });
    const group = rendered.getByRole('group', { name: 'Thread participants (1)' });
    await group.getByRole('button', { name: 'Alice', exact: true }).hover();
    await expect.element(page.getByRole('tooltip').filter({ hasText: 'Alice' })).toBeVisible();

    liveParticipantProfiles.set({ Alice: { displayName: 'Alicia', login: 'new-login' } });
    await expect.element(group.getByRole('button', { name: 'Alicia', exact: true })).toBeVisible();
    await expect.element(page.getByRole('tooltip').filter({ hasText: 'Alicia' })).toBeVisible();

    liveParticipantProfiles.set({ Alice: { displayName: '', login: 'new-login' } });
    await expect
      .element(group.getByRole('button', { name: 'new-login', exact: true }))
      .toBeVisible();
    await expect.element(page.getByRole('tooltip').filter({ hasText: 'new-login' })).toBeVisible();
  });

  it('opens the participant menu on right-click and clears it when the participant disappears', async () => {
    mocks.threadStore!.threadEvents = [participantRoot(['Alice', 'Bob'])];
    const rendered = render(ThreadPane, { props: threadProps });
    const group = rendered.getByRole('group', { name: 'Thread participants (2)' });
    const avatar = group.getByRole('button', { name: 'Bob', exact: true });
    await avatar.click({ button: 'right' });
    const menu = page.getByRole('dialog', { name: 'User profile' });
    await expect.element(menu).toBeVisible();
    await expect.element(menu.getByText('@Bob', { exact: true })).toBeVisible();
    await expect.element(avatar).toHaveAttribute('aria-expanded', 'true');

    mocks.threadStore!.threadEvents = [participantRoot(['Alice'])];
    await expect.element(menu).not.toBeInTheDocument();
  });

  it('closes the participant menu when switching threads even if the user is in both', async () => {
    mocks.threadStore!.threadEvents = [participantRoot(['Alice'])];
    const other = new ThreadPaneTestStore();
    other.threadEvents = [{ ...participantRoot(['Alice']), id: 'thread-2' }];
    threadStores.set('thread-2', other);
    const rendered = render(ThreadPane, { props: threadProps });
    await rendered.getByRole('group').getByRole('button', { name: 'Alice', exact: true }).click();
    const menu = page.getByRole('dialog', { name: 'User profile' });
    await expect.element(menu).toBeVisible();
    await rendered.rerender({ ...threadProps, threadRootEventId: 'thread-2' });
    await expect.element(menu).not.toBeInTheDocument();
  });

  it('does not show participants from a reply when the root is unavailable', async () => {
    mocks.threadStore!.threadEvents = [{ ...participantRoot(['Alice']), id: 'reply' }];
    const rendered = render(ThreadPane, { props: threadProps });
    expect(
      rendered.container.querySelector('[data-testid="thread-header-participants"]')
    ).toBeNull();
  });

  it('keeps a large participant count and header controls inside a narrow pane', async () => {
    await page.viewport(320, 480);
    try {
      mocks.threadStore!.threadEvents = [participantRoot(['Alice', 'Bob', 'Carol', 'Dave'], 10000)];
      const rendered = render(ThreadPane, {
        props: { ...threadProps, roomName: 'a-very-long-channel-name-for-the-header' }
      });
      const group = rendered.getByRole('group', { name: 'Thread participants (10000)' });
      await expect.element(group.getByText('+9997')).toBeVisible();
      const header = q(rendered.container, '[data-page-reveal]')!;
      for (const button of header.querySelectorAll('button')) {
        if (!button.getClientRects().length) continue;
        const rect = button.getBoundingClientRect();
        expect(rect.left).toBeGreaterThanOrEqual(0);
        expect(rect.right).toBeLessThanOrEqual(320);
      }
      expect(header.scrollWidth).toBeLessThanOrEqual(320);
    } finally {
      await page.viewport(1280, 720);
    }
  });

  it('adapts the participant preview to pane width as it resizes', async () => {
    mocks.threadStore!.threadEvents = [
      participantRoot(
        Array.from({ length: 20 }, (_, index) => `User ${index}`),
        10000
      )
    ];
    const rendered = render(ThreadPane, { props: threadProps });
    const group = rendered.getByRole('group', { name: 'Thread participants (10000)' });
    const pane = q(rendered.container, '[data-testid="thread-pane"]')!;
    try {
      await page.viewport(1280, 720);
      // Resize the pane within a fixed desktop viewport, as the split-pane handle does.
      for (const [width, count] of [
        [640, 12],
        [480, 8],
        [360, 5],
        [320, 3]
      ]) {
        pane.style.width = `${width}px`;
        await expect.element(group.getByText(`+${10000 - count}`)).toBeVisible();
        expect(group.element().querySelectorAll('button')).toHaveLength(count);
        const header = q(rendered.container, '[data-page-reveal]')!;
        expect(header.scrollWidth).toBeLessThanOrEqual(width);
      }
    } finally {
      await page.viewport(1280, 720);
    }
  });

  it('updates the composer when interaction posting is granted and revoked', async () => {
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose,
        canPostInThread: false
      }
    });
    const send = q(container, '[data-testid="thread-composer-send"]') as HTMLButtonElement;
    await expect.element(send).toBeDisabled();
    mocks.threadStore!.threadEvents = [
      {
        id: 'thread-root',
        createdAt: '2026-09-20T12:00:00Z',
        event: {
          kind: TimelineEventKind.MessagePosted,
          roomId: 'room-1',
          body: 'Related thread',
          attachments: [],
          reactions: [],
          replyCount: 0,
          threadParticipants: [],
          canReplyInThread: true
        }
      }
    ];
    await expect.element(send).toBeEnabled();
    const root = mocks.threadStore!.threadEvents[0].event;
    if (root.kind !== TimelineEventKind.MessagePosted) throw new Error('Expected message');
    root.canReplyInThread = false;
    await expect.element(send).toBeDisabled();
  });

  it('reveals a different thread without remounting the pane or replaying for content updates', async () => {
    const rendered = render(ThreadPane, { props: threadProps });
    await tick();
    const pane = q(rendered.container, '[data-testid="thread-pane"]') as HTMLElement;
    const conversation = q(rendered.container, '[data-testid="thread-conversation-pane"]');
    const animate = vi.spyOn(pane, 'animate');
    try {
      await rendered.rerender({ ...threadProps, threadRootEventId: 'thread-2' });
      expect(animate).toHaveBeenCalledOnce();
      expect(animate.mock.calls[0][1]).toMatchObject({ duration: 140 });
      expect(q(rendered.container, '[data-testid="thread-pane"]')).toBe(pane);
      expect(q(rendered.container, '[data-testid="thread-conversation-pane"]')).toBe(conversation);

      await rendered.rerender({
        ...threadProps,
        threadRootEventId: 'thread-2',
        roomName: 'Renamed',
        presentation: 'split'
      });
      mocks.threadStore!.threadEvents = [...mocks.threadStore!.threadEvents];
      await tick();
      expect(animate).toHaveBeenCalledOnce();
    } finally {
      animate.mockRestore();
    }
  });

  it('uses the persisted width and accessible resize handle in split layouts', async () => {
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose,
        presentation: 'split'
      }
    });

    const pane = q(container, '[data-testid="thread-pane"]') as HTMLElement;
    const handle = q(container, '[role="slider"][aria-label^="Resize:"]') as HTMLElement;

    expect(pane.className).toContain('relative');
    expect(pane.style.getPropertyValue('--thread-pane-width')).toBe('420px');

    handle.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true }));

    await vi.waitFor(() => {
      expect(pane.style.getPropertyValue('--thread-pane-width')).toBe(`${THREAD_PANE_MAX_WIDTH}px`);
      expect(handle.getAttribute('aria-valuenow')).toBe(String(THREAD_PANE_MAX_WIDTH));
    });
    expect(localStorage.getItem('chatto:threadPaneWidth')).toBe(String(THREAD_PANE_MAX_WIDTH));
  });

  it('uses the overlay by default and hides the resize handle', () => {
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    const pane = q(container, '[data-testid="thread-pane"]') as HTMLElement;
    expect(pane.className).toContain('absolute');
    expect(pane.className).toContain('inline-end-overlay-shadow');
    expect(pane.className).toContain('lg:w-[90%]');
    expect(pane.className).not.toContain('sm:w-[90%]');
    expect(container.querySelector('[role="slider"]')).toBeNull();
  });

  it('waits for the viewer to be verified before marking the thread as read', async () => {
    const viewer = server.currentUser.user!;
    server.currentUser.invalidateVerification();
    render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });
    await tick();
    expect(mocks.canMarkThreadAsRead?.()).toBe(false);

    server.currentUser.accept(viewer);

    expect(mocks.canMarkThreadAsRead?.()).toBe(true);
  });

  it.each([null, '2026-07-04T13:00:00Z'])(
    'reconciles a read with previous position %s',
    async (previousLastReadAt) => {
      mocks.markThreadAsRead.mockResolvedValue({
        previousLastReadAt,
        lastReadAt: '2026-07-04T13:00:00Z'
      });
      render(ThreadPane, {
        props: {
          roomId: 'room-1',
          roomName: 'General',
          threadRootEventId: 'thread-root',
          onClose: mocks.onClose
        }
      });

      await vi.waitFor(() =>
        expect(mocks.markThreadAsRead).toHaveBeenCalledWith(
          {
            roomId: 'room-1',
            threadRootEventId: 'thread-root',
            upToEventId: undefined
          },
          { signal: expect.any(AbortSignal) }
        )
      );

      expect(mocks.reconcileThreadRead).toHaveBeenCalledWith('room-1', 'thread-root');
    }
  );

  it.each(['user-1', 'test-user'])('marks a visible thread arrival by %s read', async (actorId) => {
    render(ThreadPane, { props: threadProps });
    await tick();
    mocks.markThreadAsRead.mockClear();
    mocks.projectionEventHandler?.(
      new RealtimeProjectionUpdate({
        event: new PublicRealtimeEvent({
          id: 'visible-reply',
          actorId,
          event: {
            case: 'messagePosted',
            value: new MessagePostedEvent({ roomId: 'room-1', threadRootEventId: 'thread-root' })
          }
        })
      })
    );
    expect(mocks.markThreadAsRead).toHaveBeenCalledExactlyOnceWith(
      { roomId: 'room-1', threadRootEventId: 'thread-root', upToEventId: 'visible-reply' },
      { signal: expect.any(AbortSignal) }
    );
  });

  describe('root reaction reads', () => {
    beforeEach(() => {
      mocks.realUnreadLifecycle = true;
    });

    it('reads existing and arriving root reactions without reading other room activity', async () => {
      reactionOccurrences.set([
        rootReaction('existing'),
        rootReaction('other-root', { eventId: 'other-root' }),
        rootReaction('other-room', { room: { id: 'room-2', name: 'Other' } }),
        rootReaction('reply', { eventId: 'reply', threadRootId: 'thread-root' }),
        rootReaction('mention', { signalKind: NotificationSignalKind.DIRECT_MENTION }),
        rootReaction('read', { unread: false })
      ]);
      render(ThreadPane, { props: threadProps });
      await tick();
      expect(mocks.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('existing');
      mocks.markThreadAsRead.mockClear();

      reactionOccurrences.set([...reactionOccurrences.get(), rootReaction('arrival')]);
      notifyReactionUpdate();
      await tick();
      expect(mocks.markOccurrenceRead.mock.calls).toEqual([['existing'], ['arrival']]);
      expect(mocks.markThreadAsRead).not.toHaveBeenCalled();
      expect(mocks.markArrivalWhileAway).not.toHaveBeenCalled();
      expect(mocks.removeTypingUser).not.toHaveBeenCalled();
    });

    it('waits for focus before reading root reactions', async () => {
      mocks.appState.isPresent = false;
      reactionOccurrences.set([rootReaction('reaction')]);
      render(ThreadPane, { props: threadProps });
      await tick();
      expect(mocks.markOccurrenceRead).not.toHaveBeenCalled();
      mocks.appState.isPresent = true;
      await tick();
      expect(mocks.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('reaction');
    });

    it('waits for a verified viewer before reading root reactions', async () => {
      const viewer = server.currentUser.user!;
      server.currentUser.invalidateVerification();
      reactionOccurrences.set([rootReaction('reaction')]);
      render(ThreadPane, { props: threadProps });
      await tick();
      expect(mocks.markOccurrenceRead).not.toHaveBeenCalled();
      server.currentUser.accept(viewer);
      await tick();
      expect(mocks.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('reaction');
    });

    it('does not read root reactions when message reading is denied', async () => {
      reactionOccurrences.set([rootReaction('reaction')]);
      render(ConversationPane, {
        props: {
          roomId: 'room-1',
          threadRootEventId: 'thread-root',
          messageStore: server.scope.store.rooms.thread('room-1', 'thread-root'),
          canPost: false,
          canAttach: false,
          canReadMessages: false
        }
      });
      await tick();
      expect(mocks.markOccurrenceRead).not.toHaveBeenCalled();
    });

    it('reads arriving root reactions only after returning from history to the latest position', async () => {
      const { container } = render(ThreadPane, { props: threadProps });
      await tick();
      (q(container, '[data-testid="report-history-position"]') as HTMLButtonElement).click();
      await tick();
      reactionOccurrences.set([rootReaction('reaction')]);
      notifyReactionUpdate();
      await tick();
      expect(mocks.markOccurrenceRead).not.toHaveBeenCalled();
      (q(container, '[data-testid="report-latest-position"]') as HTMLButtonElement).click();
      await tick();
      expect(mocks.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('reaction');
    });

    it('waits until a pending jump finishes before reading root reactions', async () => {
      reactionOccurrences.set([rootReaction('reaction')]);
      const rendered = render(ThreadPane, {
        props: {
          ...threadProps,
          highlight: {
            roomId: 'room-1',
            threadRootEventId: 'thread-root',
            eventId: 'thread-root',
            notificationId: null
          }
        }
      });
      await tick();
      expect(mocks.markOccurrenceRead).not.toHaveBeenCalled();
      await rendered.rerender({ ...threadProps, highlight: null });
      expect(mocks.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('reaction');
    });

    it('does not duplicate a pending root read across focus changes or notification updates', async () => {
      let resolve!: () => void;
      // The store marks the occurrence read before the request settles.
      mocks.markOccurrenceRead.mockImplementationOnce(() => {
        reactionOccurrences.set([rootReaction('reaction', { unread: false })]);
        return new Promise<void>((done) => (resolve = done));
      });
      reactionOccurrences.set([rootReaction('reaction')]);
      render(ThreadPane, { props: threadProps });
      await tick();
      mocks.appState.isPresent = false;
      await tick();
      mocks.appState.isPresent = true;
      notifyReactionUpdate();
      await tick();
      expect(mocks.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('reaction');
      resolve();
      await tick();
    });

    it('retries a failed root read on focus return without looping on restored unread state', async () => {
      const error = vi.spyOn(console, 'error').mockImplementation(() => {});
      try {
        mocks.markOccurrenceRead.mockImplementationOnce(async () => {
          reactionOccurrences.set([rootReaction('reaction', { unread: false })]);
          await tick();
          reactionOccurrences.set([rootReaction('reaction')]);
          throw new Error('Read failed');
        });
        reactionOccurrences.set([rootReaction('reaction')]);
        render(ThreadPane, { props: threadProps });
        await tick();
        await expect.poll(() => error.mock.calls.length).toBe(1);
        reactionOccurrences.set([rootReaction('reaction')]);
        notifyReactionUpdate();
        await tick();
        expect(mocks.markOccurrenceRead).toHaveBeenCalledTimes(1);
        mocks.appState.isPresent = false;
        await tick();
        mocks.appState.isPresent = true;
        await tick();
        expect(mocks.markOccurrenceRead).toHaveBeenCalledTimes(2);
      } finally {
        error.mockRestore();
      }
    });

    it('reads only the new root after thread navigation', async () => {
      reactionOccurrences.set([
        rootReaction('first'),
        rootReaction('second', { eventId: 'thread-2' })
      ]);
      const rendered = render(ThreadPane, { props: threadProps });
      await tick();
      expect(mocks.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('first');
      await rendered.rerender({ ...threadProps, threadRootEventId: 'thread-2' });
      expect(mocks.markOccurrenceRead.mock.calls).toEqual([['first'], ['second']]);
    });
  });

  it('does not mark a thread read merely because the send response arrived', async () => {
    const { container } = render(ThreadPane, { props: threadProps });
    await tick();
    mocks.markThreadAsRead.mockClear();
    (q(container, '[data-testid="thread-composer-send"]') as HTMLButtonElement).click();
    await tick();
    expect(mocks.ingestEvent).toHaveBeenCalledWith(expect.objectContaining({ id: 'sent-reply' }));
    expect(mocks.markThreadAsRead).not.toHaveBeenCalled();
  });

  it.each(['user-1', 'test-user'])(
    'leaves a thread arrival by %s unread in older history',
    async (actorId) => {
      const { container } = render(ThreadPane, { props: threadProps });
      await tick();
      (q(container, '[data-testid="report-history-position"]') as HTMLButtonElement).click();
      await tick();
      mocks.markThreadAsRead.mockClear();
      mocks.projectionEventHandler?.(
        new RealtimeProjectionUpdate({
          event: new PublicRealtimeEvent({
            id: 'newer-reply',
            actorId,
            event: {
              case: 'messagePosted',
              value: new MessagePostedEvent({ roomId: 'room-1', threadRootEventId: 'thread-root' })
            }
          })
        })
      );
      expect(mocks.markThreadAsRead).not.toHaveBeenCalled();
      expect(mocks.markArrivalWhileAway).not.toHaveBeenCalled();
    }
  );

  it('places the thread unread separator only for replies in this thread while away', async () => {
    mocks.appState.isPresent = false;
    render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });
    await tick();
    mocks.markThreadAsRead.mockClear();

    const post = (id: string, actorId: string, roomId: string, threadRootEventId: string) =>
      mocks.projectionEventHandler?.(
        new RealtimeProjectionUpdate({
          event: new PublicRealtimeEvent({
            id,
            actorId,
            event: {
              case: 'messagePosted',
              value: new MessagePostedEvent({ roomId, threadRootEventId })
            }
          })
        })
      );

    post('room-message', 'user-1', 'room-1', '');
    post('other-thread-reply', 'user-1', 'room-1', 'other-root');
    post('own-reply', 'test-user', 'room-1', 'thread-root');
    expect(mocks.markArrivalWhileAway).not.toHaveBeenCalled();

    post('reply-while-away', 'user-1', 'room-1', 'thread-root');
    expect(mocks.markArrivalWhileAway).toHaveBeenCalledExactlyOnceWith('reply-while-away');
    expect(mocks.markThreadAsRead).not.toHaveBeenCalled();
  });

  it('resets jump state when the pane switches to another thread', async () => {
    const props = {
      roomId: 'room-1',
      roomName: 'General',
      threadRootEventId: 'thread-root',
      onClose: mocks.onClose
    };
    const rendered = render(ThreadPane, { props });
    await tick();
    mocks.resetJumpState.mockClear();

    await rendered.rerender({ ...props, threadRootEventId: 'thread-2' });

    expect(mocks.resetJumpState).toHaveBeenCalledOnce();
  });

  it('registers the pane as a read view and releases the registration on unmount', () => {
    const release = vi.fn();
    mocks.registerReadView.mockReturnValue(release);
    const pane = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });
    expect(mocks.registerReadView).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootId: 'thread-root'
    });
    pane.unmount();
    expect(release).toHaveBeenCalledOnce();
  });

  it('keeps deleted-partner tombstones in threads without repeating the room notice', async () => {
    mocks.dmData = {
      currentUserId: 'test-user',
      participantIds: ['test-user', 'deleted-user'],
      participants: [
        { id: 'deleted-user', login: '', displayName: '', deleted: true, presenceStatus: 0 }
      ]
    };
    const view = render(ThreadPane, { props: threadProps });
    await expect.element(view.getByTestId('event-list-show-tombstones')).toHaveTextContent('true');
    await expect
      .element(view.getByText('The other user has been deleted.'))
      .not.toBeInTheDocument();
  });

  it('forwards unread marker state and bottom arrival to EventList', () => {
    mocks.unreadMarkerEventId = 'thread-unread';
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    expect(
      (q(container, '[data-testid="event-list-unread-after"]') as HTMLOutputElement).textContent
    ).toBe('thread-unread');

    (q(container, '[data-testid="event-list-reached-bottom"]') as HTMLButtonElement).click();
    expect(mocks.clearUnreadMarker).toHaveBeenCalledOnce();
  });

  it('returns each thread to its latest replies when the pane opens, switches, or closes', async () => {
    const rendered = render(ThreadPane, { props: threadProps });
    await vi.waitFor(() => expect(mocks.restoreLatestWindow).toHaveBeenCalledOnce());

    // Leaving the first thread and opening the second each restore a window.
    await rendered.rerender({ ...threadProps, threadRootEventId: 'thread-2' });
    expect(mocks.restoreLatestWindow).toHaveBeenCalledTimes(3);

    rendered.unmount();
    expect(mocks.restoreLatestWindow).toHaveBeenCalledTimes(4);
  });

  it('does not jump to a highlight that is cleared before it starts', async () => {
    const props = {
      roomId: 'room-1',
      roomName: 'General',
      threadRootEventId: 'thread-root',
      onClose: mocks.onClose
    };
    const rendered = render(ThreadPane, {
      props: { ...props, highlight: highlight('older-reply') }
    });

    // Clear the highlight before the pane's first tick.
    await rendered.rerender({ ...props, highlight: null });
    await tick();

    expect(mocks.storeJumpToMessage).not.toHaveBeenCalled();
    expect(mocks.jumpState?.scrollToEventId).toBeNull();
  });

  it('updates the thread follow button optimistically while the RPC is pending', async () => {
    let resolveFollow!: (value: {
      following: boolean;
      state: { roomId: string; threadRootEventId: string; following: boolean };
    }) => void;
    mocks.followThread.mockReturnValue(
      new Promise((resolve) => {
        resolveFollow = resolve;
      })
    );

    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    (q(container, 'button[aria-label="Follow thread"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Unfollow thread"]')).toBeTruthy();
    });
    expect(
      (q(container, 'button[aria-label="Unfollow thread"]') as HTMLButtonElement).disabled
    ).toBe(true);
    expect(mocks.followThread).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 'thread-root'
    });

    resolveFollow({
      following: true,
      state: { roomId: 'room-1', threadRootEventId: 'thread-root', following: true }
    });

    await vi.waitFor(() => {
      expect(
        (q(container, 'button[aria-label="Unfollow thread"]') as HTMLButtonElement).disabled
      ).toBe(false);
    });
  });

  it('seeds follow state when the lazy thread root arrives after mount', async () => {
    mocks.threadStore!.isInitialLoading = true;
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    expect(q(container, 'button[aria-label="Follow thread"]')).toBeTruthy();

    mocks.threadStore!.threadEvents = [
      {
        id: 'thread-root',
        createdAt: '2026-07-17T12:00:00Z',
        actorId: 'test-user',
        actor: null,
        event: {
          kind: TimelineEventKind.MessagePosted,
          roomId: 'room-1',
          body: 'Thread root',
          attachments: [],
          linkPreview: null,
          updatedAt: null,
          inReplyTo: null,
          threadRootEventId: null,
          echoOfEventId: null,
          echoFromThreadRootEventId: null,
          channelEchoEventId: null,
          replyCount: 1,
          lastReplyAt: '2026-07-17T12:01:00Z',
          threadParticipants: [],
          viewerIsFollowingThread: true,
          reactions: []
        }
      }
    ];
    mocks.threadStore!.isInitialLoading = false;

    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Unfollow thread"]')).toBeTruthy();
    });
  });

  it('ignores another follow toggle while the first request is pending', async () => {
    let rejectFollow!: (error: Error) => void;
    mocks.followThread.mockReturnValue(
      new Promise((_, reject) => {
        rejectFollow = reject;
      })
    );

    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    (q(container, 'button[aria-label="Follow thread"]') as HTMLButtonElement).click();
    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Unfollow thread"]')).toBeTruthy();
    });
    const pendingButton = q(container, 'button[aria-label="Unfollow thread"]') as HTMLButtonElement;
    pendingButton.click();

    expect(pendingButton.disabled).toBe(true);
    expect(mocks.followThread).toHaveBeenCalledOnce();
    expect(mocks.unfollowThread).not.toHaveBeenCalled();

    rejectFollow(new Error('request failed'));

    await vi.waitFor(() => {
      expect(
        (q(container, 'button[aria-label="Follow thread"]') as HTMLButtonElement).disabled
      ).toBe(false);
    });
  });

  it('passes a reply-link jump to the thread store', async () => {
    render(ThreadPane, { props: threadProps });
    await tick();

    await expect(mocks.jumpState!.jumpToMessage('older-reply')).resolves.toBe(false);

    expect(mocks.storeJumpToMessage).toHaveBeenCalledWith('older-reply');
    expect(mocks.clearAnchor).toHaveBeenCalledOnce();
  });

  it('marks a highlighted notification read only after the thread scroll completes', async () => {
    mocks.threadStore!.threadEvents = [threadMessage('reply-1')];
    const { container } = render(ThreadPane, {
      props: { ...threadProps, highlight: highlight('reply-1', 'notification-1') }
    });

    await vi.waitFor(() => expect(mocks.jumpToMessage).toHaveBeenCalledWith('reply-1'));
    expect(mocks.markOccurrenceRead).not.toHaveBeenCalled();
    const complete = q(container, '[data-testid="complete-highlight"]') as HTMLButtonElement;
    await expect.element(complete).toBeEnabled();
    complete.click();
    await vi.waitFor(() => expect(mocks.markOccurrenceRead).toHaveBeenCalledWith('notification-1'));
  });

  it('fails a thread highlight that the store cannot load', async () => {
    const onHighlightComplete = vi.fn();
    const target = highlight('missing-reply', 'notification-1');

    render(ThreadPane, { props: { ...threadProps, highlight: target, onHighlightComplete } });

    await vi.waitFor(() => expect(onHighlightComplete).toHaveBeenCalledWith(target));
    expect(getToasts().some((toast) => toast.tone === 'error')).toBe(true);
    expect(mocks.jumpState?.scrollToEventId).toBeNull();
    expect(mocks.markOccurrenceRead).not.toHaveBeenCalled();
  });

  it('reports a thread jump that cannot land on its target', async () => {
    const onHighlightComplete = vi.fn();
    const target = highlight('reply-1');
    mocks.threadStore!.threadEvents = [threadMessage('reply-1')];
    const { container } = render(ThreadPane, {
      props: { ...threadProps, highlight: target, onHighlightComplete }
    });
    await vi.waitFor(() => expect(mocks.jumpState?.scrollToEventId).toBe('reply-1'));
    expect(getToasts()).toHaveLength(0);

    (q(container, '[data-testid="fail-highlight"]') as HTMLButtonElement).click();

    await vi.waitFor(() => expect(getToasts().some((toast) => toast.tone === 'error')).toBe(true));
    expect(onHighlightComplete).toHaveBeenCalledWith(target);
  });

  it('cancels an edit when the thread message is deleted', async () => {
    mocks.editingEventId = 'reply-1';
    mocks.threadStore!.threadEvents = [threadMessage('reply-1', '2026-07-04T12:05:00Z')];

    render(ThreadPane, { props: threadProps });

    await vi.waitFor(() => expect(mocks.cancelEdit).toHaveBeenCalledOnce());
  });

  it('does not cancel stored replies when the pane switches threads', async () => {
    const rendered = render(ThreadPane, { props: threadProps });
    await tick();
    expect(mocks.cancelReply).not.toHaveBeenCalled();

    await rendered.rerender({ ...threadProps, threadRootEventId: 'thread-2' });

    expect(mocks.cancelReply).not.toHaveBeenCalled();
  });

  it('keeps a reply queued for the next thread when the pane switches to it', async () => {
    const rendered = render(ThreadPane, { props: threadProps });
    await tick();
    const input = {
      roomId: 'room-1',
      threadRootEventId: 'thread-2',
      reply: { eventId: 'reply-2', actorDisplayName: 'Bob', excerpt: 'hi' }
    };

    await rendered.rerender({
      ...threadProps,
      threadRootEventId: 'thread-2',
      composerInput: input
    });

    await vi.waitFor(() => expect(mocks.startReply).toHaveBeenCalledOnce());
    expect(mocks.cancelReply).not.toHaveBeenCalled();
    expect(mocks.startReply).toHaveBeenCalledWith('reply-2', 'Bob', 'hi', undefined);
  });

  it('starts a queued reply once the thread composer is ready', async () => {
    const onComposerInputConsumed = vi.fn();
    const input = {
      roomId: 'room-1',
      threadRootEventId: 'thread-root',
      quote: 'quoted text',
      reply: { eventId: 'reply-1', actorDisplayName: 'Alice', excerpt: 'hello' }
    };

    render(ThreadPane, {
      props: { ...threadProps, composerInput: input, onComposerInputConsumed }
    });

    await vi.waitFor(() => expect(onComposerInputConsumed).toHaveBeenCalledWith(input));
    expect(mocks.requestInsertQuote).toHaveBeenCalledWith('quoted text');
    expect(mocks.startReply).toHaveBeenCalledWith('reply-1', 'Alice', 'hello', undefined);
  });
});
