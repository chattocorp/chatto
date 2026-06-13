import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { q } from '$lib/test-utils';
import type { RoomMember } from '$lib/state/room';
import type { RoomData } from '$lib/hooks/useRoomData.svelte';
import { PresenceStatus } from '$lib/gql/graphql';
import RoomSidebarTestHarness from './RoomSidebarTestHarness.svelte';

const queryMock = vi.hoisted(() => vi.fn());

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

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_userId: string, fallback: string) => fallback
  })
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
  });

  it('shows the exact total count and loads additional member pages', async () => {
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
      expect(buttonByText(container, 'Load more members')).toBeTruthy();
    });

    const loadMore = buttonByText(container, 'Load more members')!;
    loadMore.click();
    await tick();

    await vi.waitFor(() => {
      expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
        roomId: 'room-1',
        offset: 100
      });
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
    await vi.waitFor(() => {
      expect(buttonByText(container, 'Load more members')).toBeUndefined();
    });

    const renderedTitles = renderedMemberTitles(container);
    expect(renderedTitles).toHaveLength(142);
    for (let index = 1; index <= 142; index++) {
      expect(renderedTitles).toContain(`View profile of User ${index}`);
    }
  });

  it('keeps existing pagination state when loading another page fails', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => member(index + 1));
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    queryMock.mockResolvedValue({
      data: {
        room: null
      },
      error: new Error('network failed')
    });

    try {
      const { container } = render(RoomSidebarTestHarness, {
        props: {
          roomData: roomData(firstPage, 142, true)
        }
      });

      await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
      expect(renderedMemberTitles(container)).toHaveLength(100);

      const loadMore = buttonByText(container, 'Load more members')!;
      loadMore.click();
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
        expect(buttonByText(container, 'Load more members')).toBeTruthy();
      });
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });
});
