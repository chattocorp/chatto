import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { afterEach, describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import NotificationsPage from './+page.svelte';

import { NotificationLevel as ApiNotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import { RoomDirectoryScope } from '@chatto/api-types/api/v1/room_directory_pb';
import { q } from '$lib/test-utils';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import { defaultNotificationSoundFilters } from '$lib/audio/notificationSounds';
import { removeRegisteredServerQueries } from '$lib/query/cacheRegistry';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutation: vi.fn(),
  getServerNotificationPreference: vi.fn(),
  updateServerNotificationPreference: vi.fn(),
  updateRoomNotificationPreference: vi.fn(),
  getViewerStateViaConnect: vi.fn(),
  listRooms: vi.fn(),
  playNotificationSound: vi.fn(),
  activeServerId: 'origin',
  notificationLevels: {
    setServerPreference: vi.fn(),
    setRoomPreference: vi.fn()
  },
  serverInfo: {
    pushNotificationsEnabled: false,
    vapidPublicKey: null as string | null
  },
  pushNotifications: {
    ensureRegistered: vi.fn(),
    getPushCapability: vi.fn(),
    getPermission: vi.fn(),
    isSubscribed: vi.fn(),
    sendTestNotification: vi.fn()
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
  ensureRegistered: mocks.pushNotifications.ensureRegistered,
  getPushCapability: mocks.pushNotifications.getPushCapability,
  getPermission: mocks.pushNotifications.getPermission,
  isSubscribed: mocks.pushNotifications.isSubscribed,
  sendTestNotification: mocks.pushNotifications.sendTestNotification
}));

vi.mock('$lib/api-client/notificationPreferences', () => ({
  getServerNotificationPreference: mocks.getServerNotificationPreference,
  updateServerNotificationPreference: mocks.updateServerNotificationPreference,
  updateRoomNotificationPreference: mocks.updateRoomNotificationPreference
}));

vi.mock('$lib/api-client/viewer', () => ({
  getViewerStateViaConnect: mocks.getViewerStateViaConnect
}));

vi.mock('$lib/api-client/roomDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/roomDirectory')>();
  return {
    ...actual,
    createRoomDirectoryAPI: vi.fn(() => ({
      listRooms: mocks.listRooms
    }))
  };
});

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServerId
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: (serverId: string) => serverId === 'origin'
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    get serverId() {
      return mocks.activeServerId;
    },
    store: {
      serverInfo: mocks.serverInfo,
      notificationLevels: mocks.notificationLevels
    },
    connection: {
      queryScope: 'origin-session',
      isConnected: true,
      showConnectionLostBanner: false,
      client: {
        query: mocks.query,
        mutation: mocks.mutation,
        subscription: vi.fn()
      },
      connectBaseUrl: 'https://origin.test/api/connect',
      bearerToken: 'origin-token',
      apiConfig: {
        serverId: 'origin',
        baseUrl: 'https://origin.test/api/connect',
        bearerToken: 'origin-token'
      },
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    },
    isCurrent: () => true
  })
}));

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function setRangeValue(input: HTMLInputElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

function commitRangeValue(input: HTMLInputElement, value: string) {
  setRangeValue(input, value);
  input.dispatchEvent(new Event('change', { bubbles: true }));
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
    queryClient.clear();
    localStorage.clear();
    userPreferences.notificationSound = 'chime-up';
    userPreferences.resetNotificationSoundFilters();
    mocks.activeServerId = 'origin';
    mocks.playNotificationSound.mockClear();
    mocks.notificationLevels.setServerPreference.mockClear();
    mocks.notificationLevels.setRoomPreference.mockClear();
    mocks.serverInfo.pushNotificationsEnabled = false;
    mocks.serverInfo.vapidPublicKey = null;
    mocks.pushNotifications.ensureRegistered.mockReset();
    mocks.pushNotifications.ensureRegistered.mockResolvedValue(true);
    mocks.pushNotifications.getPermission.mockReset();
    mocks.pushNotifications.getPermission.mockReturnValue('default');
    mocks.pushNotifications.getPushCapability.mockReset();
    mocks.pushNotifications.getPushCapability.mockReturnValue('supported');
    mocks.pushNotifications.isSubscribed.mockReset();
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);
    mocks.pushNotifications.sendTestNotification.mockReset();
    mocks.pushNotifications.sendTestNotification.mockResolvedValue(true);
    mocks.query.mockReset();
    mocks.mutation.mockReset();
    mocks.getServerNotificationPreference.mockReset();
    mocks.getServerNotificationPreference.mockResolvedValue({
      level: ApiNotificationLevel.NORMAL,
      effectiveLevel: ApiNotificationLevel.NORMAL
    });
    mocks.updateServerNotificationPreference.mockReset();
    mocks.updateServerNotificationPreference.mockResolvedValue({
      level: ApiNotificationLevel.ALL_MESSAGES,
      effectiveLevel: ApiNotificationLevel.ALL_MESSAGES
    });
    mocks.updateRoomNotificationPreference.mockReset();
    mocks.updateRoomNotificationPreference.mockResolvedValue({
      level: ApiNotificationLevel.MUTED,
      effectiveLevel: ApiNotificationLevel.MUTED
    });
    mocks.getViewerStateViaConnect.mockReset();
    mocks.getViewerStateViaConnect.mockResolvedValue({
      roomNotificationPreferences: [
        {
          roomId: 'room-1',
          level: NotificationLevel.DEFAULT,
          effectiveLevel: NotificationLevel.NORMAL
        },
        {
          roomId: 'dm-1',
          level: NotificationLevel.MUTED,
          effectiveLevel: NotificationLevel.MUTED
        }
      ]
    });
    mocks.listRooms.mockReset();
    mocks.listRooms.mockResolvedValue([{ id: 'room-1', name: 'general', hasUnread: false }]);
  });

  afterEach(() => queryClient.clear());

  it('renders notification levels and sound choices from mocked state', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    await expect.element(q(container, 'h1')).toHaveTextContent('Notifications');
    await expect
      .element(q(container, '[data-testid="room-notification-general"]'))
      .toBeInTheDocument();
    expect(mocks.query).not.toHaveBeenCalled();
    expect(mocks.getServerNotificationPreference).toHaveBeenCalledWith(
      {
        serverId: 'origin',
        baseUrl: 'https://origin.test/api/connect',
        bearerToken: 'origin-token'
      },
      { signal: expect.any(AbortSignal) }
    );
    expect(mocks.getViewerStateViaConnect).toHaveBeenCalledWith(
      {
        serverId: 'origin',
        baseUrl: 'https://origin.test/api/connect',
        bearerToken: 'origin-token'
      },
      { signal: expect.any(AbortSignal) }
    );
    expect(mocks.listRooms).toHaveBeenCalledWith(RoomDirectoryScope.CHANNELS, {
      signal: expect.any(AbortSignal)
    });
    expect(container.textContent).toContain('Notification Sound');
    expect(container.textContent).toContain('Silent');
    expect(container.textContent).toContain('Simple');
    expect(container.textContent).toContain('Soft Pop');
    expect(container.textContent).toContain('Sound Shape');
    await expect.element(q(container, '[data-testid="notification-volume-filter"]')).toBeVisible();
    await expect
      .element(q(container, '[data-testid="notification-high-pass-filter"]'))
      .toBeVisible();
    await expect
      .element(q(container, '[data-testid="notification-low-pass-filter"]'))
      .toBeVisible();
    await expect.element(q(container, '[data-testid="notification-echo-filter"]')).toBeVisible();
    await expect.element(q(container, '[data-testid="notification-reverb-filter"]')).toBeVisible();
    await expect.element(q(container, '[data-testid="notification-crunch-filter"]')).toBeVisible();
    expect(container.querySelector('.uil--volume')).not.toBeNull();
    expect(container.querySelector('.uil--bolt')).not.toBeNull();
    expect(container.querySelector('.uil--volume-mute')).not.toBeNull();
    expect(container.querySelector('.uil--redo')).not.toBeNull();
    expect(container.querySelector('.uil--cloud')).not.toBeNull();
    expect(container.querySelector('.uil--fire')).not.toBeNull();
  });

  it('revalidates the cached notification snapshot on remount', async () => {
    const first = render(NotificationsPage);
    await settle();
    first.unmount();

    render(NotificationsPage);
    await settle();

    expect(mocks.getServerNotificationPreference).toHaveBeenCalledTimes(2);
    expect(mocks.getViewerStateViaConnect).toHaveBeenCalledTimes(2);
    expect(mocks.listRooms).toHaveBeenCalledTimes(2);
  });

  it('does not regress the shared store from a cached remount snapshot', async () => {
    const first = render(NotificationsPage);
    await settle();
    first.unmount();
    mocks.notificationLevels.setServerPreference.mockClear();
    mocks.notificationLevels.setRoomPreference.mockClear();
    const pending = deferred<never>();
    mocks.getServerNotificationPreference.mockReturnValue(pending.promise);

    const second = render(NotificationsPage);
    await settle();

    await expect
      .element(q(second.container, '[data-testid="room-notification-general"]'))
      .toBeInTheDocument();
    expect(mocks.notificationLevels.setServerPreference).not.toHaveBeenCalled();
    expect(mocks.notificationLevels.setRoomPreference).not.toHaveBeenCalled();
    second.unmount();
  });

  it('does not regress the shared store when cached remount revalidation fails', async () => {
    const first = render(NotificationsPage);
    await settle();
    first.unmount();
    mocks.notificationLevels.setServerPreference.mockClear();
    mocks.notificationLevels.setRoomPreference.mockClear();
    mocks.getServerNotificationPreference.mockRejectedValue(
      new ConnectError('preferences denied', Code.PermissionDenied)
    );

    const second = render(NotificationsPage);
    await settle();
    await settle();

    expect(second.container.textContent).toContain('preferences denied');
    expect(mocks.notificationLevels.setServerPreference).not.toHaveBeenCalled();
    expect(mocks.notificationLevels.setRoomPreference).not.toHaveBeenCalled();
    second.unmount();
  });

  it('cancels the notification snapshot when its observer unmounts', async () => {
    const pending = deferred<never>();
    mocks.getServerNotificationPreference.mockReturnValue(pending.promise);

    const view = render(NotificationsPage);
    await settle();
    const options = mocks.getServerNotificationPreference.mock.calls[0]?.[1] as {
      signal: AbortSignal;
    };

    view.unmount();

    expect(options.signal.aborted).toBe(true);
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
    expect(mocks.playNotificationSound).toHaveBeenCalledWith(
      'pop',
      defaultNotificationSoundFilters
    );
    await expect.element(softPopButton).toHaveClass(/choice-row-selected/);
  });

  it('selects silent mode without previewing a sound', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const silentButton = buttonWithText(container, 'Silent');
    silentButton.click();
    flushSync();

    expect(userPreferences.notificationSound).toBe('silent');
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
    await expect.element(silentButton).toHaveClass(/choice-row-selected/);
  });

  it('updates room notification overrides through ConnectRPC', async () => {
    mocks.getViewerStateViaConnect.mockResolvedValue({
      roomNotificationPreferences: [
        {
          roomId: 'room-1',
          level: NotificationLevel.MUTED,
          effectiveLevel: NotificationLevel.MUTED
        }
      ]
    });
    const { container } = render(NotificationsPage);
    await settle();

    const select = q(
      container,
      '[data-testid="room-notification-general"] select'
    ) as HTMLSelectElement;
    select.value = String(NotificationLevel.MUTED);
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();

    expect(mocks.updateRoomNotificationPreference).toHaveBeenCalledWith(
      {
        serverId: 'origin',
        baseUrl: 'https://origin.test/api/connect',
        bearerToken: 'origin-token'
      },
      'room-1',
      ApiNotificationLevel.MUTED
    );
    expect(mocks.mutation).not.toHaveBeenCalled();
    expect(mocks.notificationLevels.setRoomPreference).toHaveBeenLastCalledWith(
      'room-1',
      NotificationLevel.MUTED,
      NotificationLevel.MUTED
    );
  });

  it('fences a late room update after the server session is removed', async () => {
    const pending = deferred<{
      level: NotificationLevel;
      effectiveLevel: NotificationLevel;
    }>();
    mocks.updateRoomNotificationPreference.mockReturnValue(pending.promise);
    const view = render(NotificationsPage);
    await settle();
    mocks.notificationLevels.setRoomPreference.mockClear();

    const select = q(
      view.container,
      '[data-testid="room-notification-general"] select'
    ) as HTMLSelectElement;
    select.value = String(NotificationLevel.MUTED);
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();

    removeRegisteredServerQueries('origin');
    view.unmount();
    pending.resolve({
      level: NotificationLevel.MUTED,
      effectiveLevel: NotificationLevel.MUTED
    });
    await settle();

    expect(mocks.notificationLevels.setRoomPreference).not.toHaveBeenCalled();
  });

  it('cancels stale remount revalidation before updating a room preference', async () => {
    const first = render(NotificationsPage);
    await settle();
    first.unmount();
    const pending = deferred<never>();
    mocks.getServerNotificationPreference.mockReturnValueOnce(pending.promise);

    const second = render(NotificationsPage);
    await settle();
    const signal = (
      mocks.getServerNotificationPreference.mock.calls[1]?.[1] as { signal: AbortSignal }
    ).signal;
    const select = q(
      second.container,
      '[data-testid="room-notification-general"] select'
    ) as HTMLSelectElement;
    select.value = String(NotificationLevel.MUTED);
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();

    expect(signal.aborted).toBe(true);
    expect(mocks.updateRoomNotificationPreference).toHaveBeenCalledOnce();
    await vi.waitFor(() => {
      expect(mocks.getServerNotificationPreference).toHaveBeenCalledTimes(3);
    });
    second.unmount();
  });

  it('resumes remount revalidation after a failed room preference update', async () => {
    const first = render(NotificationsPage);
    await settle();
    first.unmount();
    const pending = deferred<never>();
    mocks.getServerNotificationPreference.mockReturnValueOnce(pending.promise);
    mocks.updateRoomNotificationPreference.mockRejectedValue(
      new ConnectError('update denied', Code.PermissionDenied)
    );

    const second = render(NotificationsPage);
    await settle();
    const signal = (
      mocks.getServerNotificationPreference.mock.calls[1]?.[1] as { signal: AbortSignal }
    ).signal;
    const select = q(
      second.container,
      '[data-testid="room-notification-general"] select'
    ) as HTMLSelectElement;
    select.value = String(NotificationLevel.MUTED);
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();
    await settle();

    expect(signal.aborted).toBe(true);
    await vi.waitFor(() => {
      expect(mocks.getServerNotificationPreference).toHaveBeenCalledTimes(3);
    });
    await vi.waitFor(() => {
      flushSync();
      expect(select.disabled).toBe(false);
    });
    second.unmount();
  });

  it('updates server notification level through ConnectRPC', async () => {
    const { container } = render(NotificationsPage);
    await settle();
    mocks.getServerNotificationPreference.mockResolvedValue({
      level: ApiNotificationLevel.ALL_MESSAGES,
      effectiveLevel: ApiNotificationLevel.ALL_MESSAGES
    });

    buttonWithText(container, 'All Messages').click();
    await settle();

    expect(mocks.updateServerNotificationPreference).toHaveBeenCalledWith(
      {
        serverId: 'origin',
        baseUrl: 'https://origin.test/api/connect',
        bearerToken: 'origin-token'
      },
      ApiNotificationLevel.ALL_MESSAGES
    );
    expect(mocks.mutation).not.toHaveBeenCalled();
    await vi.waitFor(() => {
      expect(mocks.notificationLevels.setServerPreference).toHaveBeenCalledWith(
        NotificationLevel.ALL_MESSAGES,
        NotificationLevel.ALL_MESSAGES
      );
    });
  });

  it('resumes remount revalidation after a failed server preference update', async () => {
    const first = render(NotificationsPage);
    await settle();
    first.unmount();
    const pending = deferred<never>();
    mocks.getServerNotificationPreference.mockReturnValueOnce(pending.promise);
    mocks.updateServerNotificationPreference.mockRejectedValue(
      new ConnectError('server update denied', Code.PermissionDenied)
    );

    const second = render(NotificationsPage);
    await settle();
    const signal = (
      mocks.getServerNotificationPreference.mock.calls[1]?.[1] as { signal: AbortSignal }
    ).signal;
    const allMessages = buttonWithText(second.container, 'All Messages');
    allMessages.click();
    await settle();

    expect(signal.aborted).toBe(true);
    await vi.waitFor(() => {
      expect(mocks.getServerNotificationPreference).toHaveBeenCalledTimes(3);
    });
    await vi.waitFor(() => {
      flushSync();
      expect(allMessages.disabled).toBe(false);
    });
    second.unmount();
  });

  it('serializes room changes through server preference reconciliation', async () => {
    const pending = deferred<{
      level: NotificationLevel;
      effectiveLevel: NotificationLevel;
    }>();
    mocks.updateServerNotificationPreference.mockReturnValue(pending.promise);
    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'All Messages').click();
    flushSync();
    const select = q(
      container,
      '[data-testid="room-notification-general"] select'
    ) as HTMLSelectElement;
    expect(select.disabled).toBe(true);
    select.value = String(NotificationLevel.DEFAULT);
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();

    expect(mocks.updateRoomNotificationPreference).not.toHaveBeenCalled();

    pending.resolve({
      level: NotificationLevel.ALL_MESSAGES,
      effectiveLevel: NotificationLevel.ALL_MESSAGES
    });
    mocks.getServerNotificationPreference.mockResolvedValue({
      level: ApiNotificationLevel.ALL_MESSAGES,
      effectiveLevel: ApiNotificationLevel.ALL_MESSAGES
    });
    await settle();
    await settle();

    expect(select.disabled).toBe(false);
    select.value = String(NotificationLevel.MUTED);
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();
    expect(mocks.updateRoomNotificationPreference).toHaveBeenCalledOnce();
  });

  it('shows the push enable path when configured and not subscribed', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);

    const { container } = render(NotificationsPage);
    await settle();

    expect(container.textContent).toContain('Push Notifications');
    await expect.element(buttonWithText(container, 'Enable')).toBeVisible();
    expect(container.textContent).not.toContain('Disable');
  });

  it('does not offer native push registration for remote servers', async () => {
    mocks.activeServerId = 'remote';
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);

    const { container } = render(NotificationsPage);
    await settle();

    expect(container.textContent).toContain('Push Notifications');
    expect(container.textContent).toContain('Native push is unavailable for remote servers');
    expect(container.textContent).toContain('In-app notification badges and sounds still work');
    expect(container.textContent).not.toContain('Get notified about new messages while Chatto');
    expect(
      Array.from(container.querySelectorAll('button')).some((button) =>
        button.textContent?.includes('Enable')
      )
    ).toBe(false);
    expect(mocks.pushNotifications.isSubscribed).not.toHaveBeenCalled();
    expect(mocks.pushNotifications.ensureRegistered).not.toHaveBeenCalled();
  });

  it('shows iOS Home Screen guidance without checking or registering push', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.getPushCapability.mockReturnValue('ios_home_screen_required');
    mocks.pushNotifications.getPermission.mockReturnValue(null);

    const { container } = render(NotificationsPage);
    await settle();

    expect(container.textContent).toContain('Push Notifications');
    expect(container.textContent).toContain('Add Chatto to your Home Screen');
    expect(container.textContent).toContain('supported iOS/iPadOS versions');
    expect(container.textContent).toContain('open it from the app icon');
    expect(container.textContent).not.toContain('Get notified about new messages while Chatto');
    expect(
      Array.from(container.querySelectorAll('button')).some((button) =>
        button.textContent?.includes('Enable')
      )
    ).toBe(false);
    expect(mocks.pushNotifications.isSubscribed).not.toHaveBeenCalled();
    expect(mocks.pushNotifications.ensureRegistered).not.toHaveBeenCalled();
  });

  it('enables push notifications through the registration helper', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);
    mocks.pushNotifications.ensureRegistered.mockImplementation(async () => {
      mocks.pushNotifications.getPermission.mockReturnValue('granted');
      return true;
    });

    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'Enable').click();
    await settle();

    expect(mocks.pushNotifications.ensureRegistered).toHaveBeenCalledWith('vapid-key', {
      prompt: true
    });
    expect(container.textContent).toContain('Push notifications enabled');
    expect(container.textContent).toContain('disable them for this site');
    expect(container.textContent).not.toContain('Disable');
  });

  it('sends a test push notification when push is enabled', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.getPermission.mockReturnValue('granted');
    mocks.pushNotifications.isSubscribed.mockResolvedValue(true);

    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'Send test notification').click();
    await settle();

    expect(mocks.pushNotifications.sendTestNotification).toHaveBeenCalledOnce();
    expect(container.textContent).toContain('Test notification sent.');
  });

  it('updates and persists notification sound filter sliders', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    setRangeValue(
      q(container, '[data-testid="notification-volume-filter"]') as HTMLInputElement,
      '1.5'
    );
    setRangeValue(
      q(container, '[data-testid="notification-high-pass-filter"]') as HTMLInputElement,
      '500'
    );
    setRangeValue(
      q(container, '[data-testid="notification-low-pass-filter"]') as HTMLInputElement,
      '63'
    );
    setRangeValue(
      q(container, '[data-testid="notification-echo-filter"]') as HTMLInputElement,
      '35'
    );
    setRangeValue(
      q(container, '[data-testid="notification-reverb-filter"]') as HTMLInputElement,
      '45'
    );
    setRangeValue(
      q(container, '[data-testid="notification-crunch-filter"]') as HTMLInputElement,
      '55'
    );

    expect(userPreferences.notificationSoundFilters).toEqual({
      volume: 1.5,
      highPassHz: 500,
      lowPassHz: 7904,
      echo: 35,
      reverb: 45,
      crunch: 55
    });
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      notificationSoundFilters: {
        volume: 1.5,
        highPassHz: 500,
        lowPassHz: 7904,
        echo: 35,
        reverb: 45,
        crunch: 55
      }
    });
    expect(container.textContent).toContain('150%');
    expect(container.textContent).toContain('Tinny');
    expect(container.textContent).toContain('24%');
    expect(container.textContent).toContain('Muffled');
    expect(container.textContent).toContain('63%');
    expect(container.textContent).toContain('Echo');
    expect(container.textContent).toContain('35%');
    expect(container.textContent).toContain('Reverb');
    expect(container.textContent).toContain('45%');
    expect(container.textContent).toContain('Crunch');
    expect(container.textContent).toContain('55%');
  });

  it('previews the selected sound with the current filters', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    setRangeValue(
      q(container, '[data-testid="notification-high-pass-filter"]') as HTMLInputElement,
      '400'
    );
    mocks.playNotificationSound.mockClear();

    buttonWithText(container, 'Preview').click();
    flushSync();

    expect(mocks.playNotificationSound).toHaveBeenCalledWith('chime-up', {
      ...defaultNotificationSoundFilters,
      highPassHz: 400
    });
  });

  it('previews a filter change only when the slider change is committed', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const volumeInput = q(
      container,
      '[data-testid="notification-volume-filter"]'
    ) as HTMLInputElement;
    mocks.playNotificationSound.mockClear();

    setRangeValue(volumeInput, '1.25');
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();

    volumeInput.dispatchEvent(new Event('change', { bubbles: true }));
    flushSync();

    expect(mocks.playNotificationSound).toHaveBeenCalledOnce();
    expect(mocks.playNotificationSound).toHaveBeenCalledWith('chime-up', {
      ...defaultNotificationSoundFilters,
      volume: 1.25
    });
  });

  it('does not preview committed filter changes while Silent is selected', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'Silent').click();
    flushSync();
    mocks.playNotificationSound.mockClear();

    commitRangeValue(
      q(container, '[data-testid="notification-echo-filter"]') as HTMLInputElement,
      '60'
    );

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('disables preview when silent is selected', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'Silent').click();
    flushSync();
    mocks.playNotificationSound.mockClear();

    const previewButton = buttonWithText(container, 'Preview');
    expect(previewButton.disabled).toBe(true);
    previewButton.click();
    flushSync();

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('resets notification sound filters to defaults', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    setRangeValue(
      q(container, '[data-testid="notification-volume-filter"]') as HTMLInputElement,
      '0.5'
    );
    buttonWithText(container, 'Reset').click();
    flushSync();

    expect(userPreferences.notificationSoundFilters).toEqual(defaultNotificationSoundFilters);
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      notificationSoundFilters: defaultNotificationSoundFilters
    });
    expect(container.textContent).toContain('100%');
  });
});
