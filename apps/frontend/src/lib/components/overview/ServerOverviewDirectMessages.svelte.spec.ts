import '../../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { RoomsListItem } from '$lib/state/server/navigation';
import ServerOverviewDirectMessages from './ServerOverviewDirectMessages.svelte';

const member = (id: string, displayName: string) => ({
  id,
  login: id,
  displayName,
  deleted: false,
  isBot: false,
  avatarUrl: null,
  presenceStatus: PresenceStatus.OFFLINE
});

function dm(id: string, overrides: Partial<RoomsListItem> = {}): RoomsListItem {
  return {
    id,
    name: '',
    type: RoomKind.DM,
    isUniversal: false,
    viewerIsMember: true,
    viewerCanJoinRoom: false,
    viewerCanManageRoom: false,
    viewerNotificationCount: 0,
    viewerImportantNotificationCount: 0,
    members: [member('me', 'Me'), member('ada', 'Ada')],
    ...overrides
  };
}

describe('ServerOverviewDirectMessages', () => {
  it('lists conversations by partner and opens notifications from the badge', async () => {
    const onopennotification = vi.fn();
    render(ServerOverviewDirectMessages, {
      conversations: [
        dm('dm-ada', { viewerNotificationCount: 2, viewerImportantNotificationCount: 2 })
      ],
      serverId: 'origin',
      viewerId: 'me',
      isUnread: () => true,
      onopennotification
    });

    await expect.element(page.getByRole('link', { name: /Ada/ })).toBeVisible();
    await page.getByRole('button', { name: /2/ }).click();
    expect(onopennotification).toHaveBeenCalledWith('dm-ada', NotificationAttentionLevel.IMPORTANT);
  });

  it('marks unread conversations without notifications', async () => {
    render(ServerOverviewDirectMessages, {
      conversations: [dm('dm-ada')],
      serverId: 'origin',
      viewerId: 'me',
      isUnread: () => true,
      onopennotification: vi.fn()
    });

    await expect.element(page.getByText('Unread messages')).toBeInTheDocument();
  });

  it('renders nothing without conversations', () => {
    const { container } = render(ServerOverviewDirectMessages, {
      conversations: [],
      serverId: 'origin',
      viewerId: 'me',
      isUnread: () => false,
      onopennotification: vi.fn()
    });
    expect(container.querySelector('[data-testid="server-overview-direct-messages"]')).toBeNull();
  });
});
