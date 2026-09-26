import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import type { MessagesStore } from '$lib/state/room';
import { q } from '$lib/test-utils';
import RoomEventsPane from './RoomEventsPane.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    editState: {
      eventId: null as string | null,
      cancelEdit: vi.fn()
    },
    jumpState: {
      scrollToEventId: null as string | null,
      setJumpHandler: vi.fn(),
      setLoadNewerHandler: vi.fn(),
      reset: vi.fn()
    }
  }
}));

vi.mock('$lib/state/room', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/state/room')>();
  return {
    ...actual,
    getComposerContext: () => ({
      editState: mocks.editState,
      jumpState: mocks.jumpState
    })
  };
});

vi.mock('./EventList.svelte', async () => {
  const { default: EventListContractMock } = await import('./EventListContractMock.svelte');
  return { default: EventListContractMock };
});

function createStore(): MessagesStore {
  return {
    rootEvents: [],
    setRoom: vi.fn(),
    jumpToMessage: vi.fn(),
    loadNewer: vi.fn(),
    loadMore: vi.fn()
  } as unknown as MessagesStore;
}

describe('RoomEventsPane', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('uses the limited empty state and restores the normal timeline when access changes', async () => {
    const store = createStore();
    let limited = $state(true);
    const { container } = render(RoomEventsPane, {
      props: {
        roomId: 'room-1',
        messageStore: store,
        get hasLimitedMessageAccess() {
          return limited;
        }
      }
    });

    await expect
      .element(q(container, '[data-testid="event-list-empty-message"]'))
      .toHaveTextContent('No conversations you can read yet.');
    await expect
      .element(q(container, '[data-testid="event-list-start-marker"]'))
      .toHaveTextContent('false');

    flushSync(() => {
      limited = false;
    });
    await expect
      .element(q(container, '[data-testid="event-list-start-marker"]'))
      .toHaveTextContent('true');
    expect(mocks.jumpState.reset).toHaveBeenCalledOnce();
    expect(store.setRoom).toHaveBeenCalledOnce();
    expect(store.loadMore).not.toHaveBeenCalled();
  });

  it('forwards unread marker state and bottom arrival to EventList', () => {
    const onUnreadMarkerCleared = vi.fn();
    const { container } = render(RoomEventsPane, {
      props: {
        roomId: 'room-1',
        messageStore: createStore(),
        unreadMarkerEventId: 'room-unread',
        onUnreadMarkerCleared
      }
    });

    expect(
      (q(container, '[data-testid="event-list-unread-after"]') as HTMLOutputElement).textContent
    ).toBe('room-unread');

    (q(container, '[data-testid="event-list-reached-bottom"]') as HTMLButtonElement).click();
    expect(onUnreadMarkerCleared).toHaveBeenCalledOnce();
  });
});
