import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { q } from '$lib/test-utils';
import type { RoomMember } from '$lib/state/room';
import type { PresenceCache } from '$lib/state/presenceCache.svelte';
import type { RoomData } from '$lib/hooks/useRoomData.svelte';
import { PresenceStatus } from '$lib/gql/graphql';
import RoomSidebarTestHarness from './RoomSidebarTestHarness.svelte';

const queryMock = vi.hoisted(() => vi.fn());

class MockIntersectionObserver {
  static instances: MockIntersectionObserver[] = [];

  readonly callback: IntersectionObserverCallback;
  readonly elements = new Set<Element>();

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    MockIntersectionObserver.instances.push(this);
  }

  observe(element: Element) {
    this.elements.add(element);
  }

  unobserve(element: Element) {
    this.elements.delete(element);
  }

  disconnect() {
    this.elements.clear();
  }

  trigger(isIntersecting = true) {
    const entries = Array.from(this.elements).map((target) => ({
      isIntersecting,
      target
    }));
    this.callback(entries as IntersectionObserverEntry[], this as unknown as IntersectionObserver);
  }
}

vi.mock('$lib/hooks/useEvent.svelte', () => ({
  useEvent: vi.fn(),
  usePresenceChange: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: queryMock,
      mutation: vi.fn(),
      subscription: vi.fn()
    }
  })
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'test-server'
}));

vi.mock('$lib/state/server/permissions.svelte', () => ({
  getServerPermissions: () => ({
    current: {
      canStartDMs: false
    }
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveLogin: (_userId: string, fallback: string) => fallback
}));

function member(index: number): RoomMember {
  return {
    id: `user-${index}`,
    login: `user${index}`,
    displayName: `User ${index}`,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online
  };
}

function buttonByText(container: Element, text: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll('button')).find((button) =>
    button.textContent?.includes(text)
  );
}

function renderedMemberTitles(container: Element): string[] {
  return Array.from(container.querySelectorAll('[title^="View profile of User "]')).map(
    (element) => element.getAttribute('title') ?? ''
  );
}

function presenceBadge(container: Element, label: string): Element | null {
  return container.querySelector(`[aria-label="${label}"]`);
}

function roomData(members: RoomMember[], totalCount: number, hasMore: boolean): RoomData {
  return {
    room: { id: 'room-1', name: 'general', type: 'CHANNEL' },
    spaceName: 'Test Server',
    canPostMessage: true,
    canPostInThread: true,
    canReact: true,
    canManageOthersMessage: false,
    canEchoMessage: false,
    canManageRoom: false,
    canBanRoomMembers: false,
    members,
    membersTotalCount: totalCount,
    membersHasMore: hasMore
  };
}

describe('RoomSidebar', () => {
  beforeEach(() => {
    queryMock.mockReset();
    localStorage.clear();
    MockIntersectionObserver.instances = [];
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);
  });

  it('shows the exact total count and automatically loads additional member pages', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => member(index + 1));
    const secondPage = Array.from({ length: 42 }, (_, index) => member(index + 101));

    queryMock.mockResolvedValue({
      data: {
        room: {
          members: {
            users: secondPage,
            totalCount: 142,
            hasMore: false
          }
        }
      },
      error: null
    });

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData(firstPage, 142, true)
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
    expect(renderedMemberTitles(container)).toHaveLength(100);
    await vi.waitFor(() => {
      expect(
        container.querySelector('[data-testid="room-members-load-more-sentinel"]')
      ).toBeTruthy();
      expect(MockIntersectionObserver.instances).toHaveLength(1);
    });

    MockIntersectionObserver.instances[0].trigger();
    await tick();

    await vi.waitFor(() => {
      expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
        roomId: 'room-1',
        offset: 100
      });
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
    await vi.waitFor(() => {
      expect(
        container.querySelector('[data-testid="room-members-load-more-sentinel"]')
      ).toBeFalsy();
    });

    const renderedTitles = renderedMemberTitles(container);
    expect(renderedTitles).toHaveLength(142);
    for (let index = 1; index <= 142; index++) {
      expect(renderedTitles).toContain(`View profile of User ${index}`);
    }
  });

  it('keeps existing pagination state when automatic pagination fails and allows retry', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => member(index + 1));
    const secondPage = Array.from({ length: 42 }, (_, index) => member(index + 101));
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    queryMock
      .mockResolvedValueOnce({
        data: {
          room: null
        },
        error: new Error('network failed')
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: secondPage,
              totalCount: 142,
              hasMore: false
            }
          }
        },
        error: null
      });

    try {
      const { container } = render(RoomSidebarTestHarness, {
        props: {
          roomData: roomData(firstPage, 142, true)
        }
      });

      await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
      expect(renderedMemberTitles(container)).toHaveLength(100);

      await vi.waitFor(() => {
        expect(MockIntersectionObserver.instances).toHaveLength(1);
      });

      MockIntersectionObserver.instances[0].trigger();
      await tick();

      await vi.waitFor(() => {
        expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
          roomId: 'room-1',
          offset: 100
        });
      });

      await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
      expect(renderedMemberTitles(container)).toHaveLength(100);
      await vi.waitFor(() => {
        expect(
          container.querySelector('[data-testid="room-members-load-more-sentinel"]')
        ).toBeTruthy();
      });

      MockIntersectionObserver.instances[0].trigger();
      await tick();

      await vi.waitFor(() => {
        expect(queryMock).toHaveBeenCalledTimes(2);
      });

      await vi.waitFor(() => {
        expect(renderedMemberTitles(container)).toHaveLength(142);
        expect(
          container.querySelector('[data-testid="room-members-load-more-sentinel"]')
        ).toBeFalsy();
      });
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('keeps away members present while showing the global away badge', async () => {
    let presenceCache: PresenceCache | null = null;
    const [user] = [member(1)];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([user], 1, false),
        onPresenceCacheReady: (cache: PresenceCache) => {
          presenceCache = cache;
        }
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (1)');
    expect(presenceBadge(container, 'Online')).toBeTruthy();
    await vi.waitFor(() => {
      expect(buttonByText(container, 'Online (1)')).toBeTruthy();
    });

    await vi.waitFor(() => {
      expect(presenceCache).toBeTruthy();
    });
    presenceCache!.update(user.id, PresenceStatus.Away);
    await tick();

    expect(presenceBadge(container, 'Away')).toBeTruthy();
    expect(buttonByText(container, 'Online (1)')).toBeTruthy();

    presenceCache!.update(user.id, PresenceStatus.Online);
    await tick();

    expect(presenceBadge(container, 'Online')).toBeTruthy();
    expect(buttonByText(container, 'Online (1)')).toBeTruthy();
  });

  it('calls onClose when the room extras close button is clicked', async () => {
    const onClose = vi.fn();
    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([member(1)], 1, false),
        onClose
      }
    });

    const closeButton = container.querySelector(
      '[aria-label="Hide room extras"]'
    ) as HTMLButtonElement | null;
    expect(closeButton).toBeTruthy();

    closeButton!.click();
    await tick();

    expect(onClose).toHaveBeenCalledOnce();
  });

  it('renders overlay presentation without desktop resizing chrome', async () => {
    const { container } = render(RoomSidebarTestHarness, {
      props: {
        presentation: 'overlay',
        roomData: roomData([member(1)], 1, false)
      }
    });

    const sidebar = container.querySelector('[aria-label="Room extras"]') as HTMLElement | null;
    expect(sidebar).toBeTruthy();
    expect(sidebar!.style.width).toBe('');
    expect(container.querySelector('[aria-label="Resize room extras pane"]')).toBeFalsy();
  });

  it('renders the files coming soon panel', async () => {
    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'files',
        roomData: roomData([member(1)], 1, false)
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Files');
    expect(container.textContent).toContain('Files coming soon.');
    expect(container.querySelector('[aria-label="Members"]')).toBeFalsy();
  });
});
