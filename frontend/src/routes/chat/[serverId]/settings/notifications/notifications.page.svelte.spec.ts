import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import NotificationsPage from './+page.svelte';
import { NotificationLevel } from '$lib/gql/graphql';
import { q } from '$lib/test-utils';
import { userPreferences } from '$lib/state/userPreferences.svelte';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutation: vi.fn(),
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

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: mocks.query,
      mutation: mocks.mutation,
      subscription: vi.fn()
    }
  })
}));

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function preferenceResult() {
  return {
    server: {
      viewerNotificationPreference: {
        level: NotificationLevel.Normal,
        effectiveLevel: NotificationLevel.Normal
      }
    },
    viewer: {
      user: {
        rooms: [
          {
            id: 'room-1',
            name: 'general',
            viewerNotificationPreference: {
              level: NotificationLevel.Default,
              effectiveLevel: NotificationLevel.Normal
            }
          }
        ]
      }
    }
  };
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
    mocks.query.mockReset();
    mocks.query.mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({
        data: preferenceResult(),
        error: null
      })
    });
    mocks.mutation.mockReset();
  });

  it('renders notification levels and sound choices from mocked state', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    await expect.element(q(container, 'h1')).toHaveTextContent('Notifications');
    await expect
      .element(q(container, '[data-testid="room-notification-general"]'))
      .toBeInTheDocument();
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
