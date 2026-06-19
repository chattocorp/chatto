import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import NotificationsPage from './+page.svelte';
import { q } from '$lib/test-utils';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import {
  GetViewerResponse,
  ListMyRoomsResponse,
  RoomListItemView,
  Viewer,
  ViewerNotificationPreferenceView
} from '$lib/pb/chatto/api/v1/chat_pb';
import { Room, RoomKind } from '$lib/pb/chatto/core/v1/models_pb';
import { NotificationLevel as WireNotificationLevel } from '$lib/pb/chatto/core/v1/user_preferences_pb';

const mocks = vi.hoisted(() => ({
  getViewer: vi.fn(),
  listMyRooms: vi.fn(),
  setServerNotificationLevel: vi.fn(),
  setRoomNotificationLevel: vi.fn(),
  playNotificationSound: vi.fn(),
  notificationLevels: {
    setServerPreference: vi.fn(),
    setRoomPreference: vi.fn()
  }
}));

vi.mock('$lib/audio/notificationSounds', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/audio/notificationSounds')>();
  return {
    ...actual,
    playNotificationSound: mocks.playNotificationSound
  };
});

vi.mock('$lib/notifications/pushNotifications', () => ({
  isSupported: () => true,
  isSubscribed: vi.fn().mockResolvedValue(false),
  subscribe: vi.fn().mockResolvedValue(true),
  unsubscribe: vi.fn().mockResolvedValue(true)
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      serverInfo: {
        pushNotificationsEnabled: false,
        vapidPublicKey: null
      },
      notificationLevels: mocks.notificationLevels
    })
  }
}));

vi.mock('$lib/state/server/wireEventBus.svelte', () => ({
  wireEventBusManager: {
    getClient: () => ({
      getViewer: mocks.getViewer,
      listMyRooms: mocks.listMyRooms,
      setServerNotificationLevel: mocks.setServerNotificationLevel,
      setRoomNotificationLevel: mocks.setRoomNotificationLevel
    })
  }
}));

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function buttonWithText(container: Element, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll('button')).find((candidate) =>
    candidate.textContent?.includes(text)
  );
  if (!button) {
    throw new Error(`Button with text "${text}" not found`);
  }
  return button;
}

describe('Notification settings page', () => {
  beforeEach(() => {
    localStorage.clear();
    userPreferences.notificationSound = 'chime-up';
    mocks.playNotificationSound.mockClear();
    mocks.notificationLevels.setServerPreference.mockClear();
    mocks.notificationLevels.setRoomPreference.mockClear();
    mocks.getViewer.mockReset();
    mocks.getViewer.mockResolvedValue(
      new GetViewerResponse({
        viewer: new Viewer({
          serverNotificationPreference: new ViewerNotificationPreferenceView({
            level: WireNotificationLevel.NORMAL,
            effectiveLevel: WireNotificationLevel.NORMAL
          })
        })
      })
    );
    mocks.listMyRooms.mockReset();
    mocks.listMyRooms.mockResolvedValue(
      new ListMyRoomsResponse({
        roomViews: [
          new RoomListItemView({
            room: new Room({
              id: 'room-1',
              name: 'general',
              kind: RoomKind.CHANNEL
            }),
            viewerNotificationPreference: new ViewerNotificationPreferenceView({
              level: WireNotificationLevel.UNSPECIFIED,
              effectiveLevel: WireNotificationLevel.NORMAL
            })
          })
        ]
      })
    );
    mocks.setServerNotificationLevel.mockReset();
    mocks.setRoomNotificationLevel.mockReset();
  });

  it('renders notification levels and sound choices from mocked state', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    await expect.element(q(container, 'h1')).toHaveTextContent('Notifications');
    await expect
      .element(q(container, '[data-testid="room-notification-general"]'))
      .toBeInTheDocument();
    expect(mocks.listMyRooms.mock.calls[0][0]?.kind).toBe(RoomKind.CHANNEL);
    expect(container.textContent).toContain('Notification Sound');
    expect(container.textContent).toContain('Silent');
    expect(container.textContent).toContain('Simple');
    expect(container.textContent).toContain('Soft Pop');
  });

  it('selects and persists a non-silent notification sound', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const softPopButton = buttonWithText(container, 'Soft Pop');
    softPopButton.click();
    flushSync();

    expect(userPreferences.notificationSound).toBe('pop');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      notificationSound: 'pop'
    });
    expect(mocks.playNotificationSound).toHaveBeenCalledWith('pop');
    await expect.element(softPopButton).toHaveClass(/border-accent/);
  });

  it('selects silent mode without previewing a sound', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const silentButton = buttonWithText(container, 'Silent');
    silentButton.click();
    flushSync();

    expect(userPreferences.notificationSound).toBe('silent');
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
    await expect.element(silentButton).toHaveClass(/border-accent/);
  });
});
