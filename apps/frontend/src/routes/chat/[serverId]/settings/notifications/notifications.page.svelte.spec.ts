import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import NotificationsPage from './+page.svelte';

import { q } from '$lib/test-utils';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import { defaultNotificationSoundFilters } from '$lib/audio/notificationSounds';

const mocks = vi.hoisted(() => ({
  playNotificationSound: vi.fn(),
  activeServerId: 'origin',
  notifications: {
    getPolicy: vi.fn().mockResolvedValue(null),
    updatePolicy: vi.fn().mockResolvedValue(null)
  },
  serverInfo: {
    pushNotificationsEnabled: false,
    vapidPublicKey: null as string | null,
    supportsFeature: vi.fn(() => true)
  },
  pushNotifications: {
    ensureRegistered: vi.fn(),
    isBrowserWebPushRuntime: vi.fn(),
    getPushCapability: vi.fn(),
    getPermission: vi.fn(),
    isSubscribed: vi.fn(),
    refreshPushSubscriptions: vi.fn(),
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
  isBrowserWebPushRuntime: mocks.pushNotifications.isBrowserWebPushRuntime,
  getPushCapability: mocks.pushNotifications.getPushCapability,
  getPermission: mocks.pushNotifications.getPermission,
  isSubscribed: mocks.pushNotifications.isSubscribed,
  refreshPushSubscriptions: mocks.pushNotifications.refreshPushSubscriptions,
  sendTestNotification: mocks.pushNotifications.sendTestNotification
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: (serverId: string) => serverId === 'origin'
  }
}));

vi.mock('$lib/state/server/scope.svelte', async () => {
  const { userPreferences: reactivePreferences } =
    await import('$lib/state/userPreferences.svelte');
  return {
    useServerScope: () => ({
      get serverId() {
        // Keep the mock route ID reactive without introducing test-only state
        // into production code.
        void reactivePreferences.notificationSound;
        return mocks.activeServerId;
      },
      store: {
        serverInfo: mocks.serverInfo,
        notifications: mocks.notifications
      },
      connection: {
        queryScope: 'origin-session',
        isConnected: true,
        showConnectionLostBanner: false,
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
  };
});

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
    localStorage.clear();
    userPreferences.notificationSound = 'chime-up';
    userPreferences.resetNotificationSoundFilters();
    mocks.activeServerId = 'origin';
    mocks.playNotificationSound.mockClear();
    mocks.notifications.getPolicy.mockClear();
    mocks.notifications.getPolicy.mockResolvedValue(null);
    mocks.notifications.updatePolicy.mockClear();
    mocks.notifications.updatePolicy.mockResolvedValue(null);
    mocks.serverInfo.pushNotificationsEnabled = false;
    mocks.serverInfo.vapidPublicKey = null;
    mocks.serverInfo.supportsFeature.mockReturnValue(true);
    mocks.pushNotifications.ensureRegistered.mockReset();
    mocks.pushNotifications.ensureRegistered.mockResolvedValue(true);
    mocks.pushNotifications.isBrowserWebPushRuntime.mockReset();
    mocks.pushNotifications.isBrowserWebPushRuntime.mockReturnValue(true);
    mocks.pushNotifications.getPermission.mockReset();
    mocks.pushNotifications.getPermission.mockReturnValue('default');
    mocks.pushNotifications.getPushCapability.mockReset();
    mocks.pushNotifications.getPushCapability.mockReturnValue('supported');
    mocks.pushNotifications.isSubscribed.mockReset();
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);
    mocks.pushNotifications.refreshPushSubscriptions.mockReset();
    mocks.pushNotifications.refreshPushSubscriptions.mockResolvedValue(undefined);
    mocks.pushNotifications.sendTestNotification.mockReset();
    mocks.pushNotifications.sendTestNotification.mockResolvedValue(true);
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

  it('offers an independent push subscription for remote servers', async () => {
    mocks.activeServerId = 'remote';
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);

    const { container } = render(NotificationsPage);
    await settle();

    expect(container.textContent).toContain('Push Notifications');
    await expect.element(buttonWithText(container, 'Enable')).toBeVisible();
    expect(mocks.pushNotifications.isSubscribed).toHaveBeenCalledWith('remote');
  });

  it('does not offer browser Web Push controls inside Chatto Desktop', async () => {
    mocks.activeServerId = 'remote';
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isBrowserWebPushRuntime.mockReturnValue(false);

    const { container } = render(NotificationsPage);
    await settle();

    expect(container.textContent).not.toContain('Push Notifications');
    expect(mocks.pushNotifications.isSubscribed).not.toHaveBeenCalled();
  });

  it('ignores a stale subscription result after navigating to another server', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    const originResult = deferred<boolean>();
    mocks.pushNotifications.isSubscribed
      .mockReturnValueOnce(originResult.promise)
      .mockResolvedValueOnce(false);

    const view = render(NotificationsPage);
    await settle();
    expect(mocks.pushNotifications.isSubscribed).toHaveBeenCalledWith('origin');

    mocks.activeServerId = 'remote';
    userPreferences.notificationSound = 'pop';
    await settle();
    expect(mocks.pushNotifications.isSubscribed).toHaveBeenCalledWith('remote');

    originResult.resolve(true);
    await settle();

    await expect.element(buttonWithText(view.container, 'Enable')).toBeVisible();
    expect(view.container.textContent).not.toContain('Push notifications enabled');
  });

  it('ignores a stale subscription result after navigating away and back', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    const firstOriginResult = deferred<boolean>();
    mocks.pushNotifications.isSubscribed
      .mockReturnValueOnce(firstOriginResult.promise)
      .mockResolvedValue(false);

    const view = render(NotificationsPage);
    await settle();

    mocks.activeServerId = 'remote';
    userPreferences.notificationSound = 'pop';
    await settle();
    mocks.activeServerId = 'origin';
    userPreferences.notificationSound = 'ding';
    await settle();

    firstOriginResult.resolve(true);
    await settle();

    await expect.element(buttonWithText(view.container, 'Enable')).toBeVisible();
    expect(view.container.textContent).not.toContain('Push notifications enabled');
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

    expect(mocks.pushNotifications.ensureRegistered).toHaveBeenCalledWith('origin', 'vapid-key', {
      prompt: true
    });
    expect(mocks.pushNotifications.refreshPushSubscriptions).toHaveBeenCalledOnce();
    expect(container.textContent).toContain('Push notifications enabled');
    expect(container.textContent).toContain('disable them for this site');
    expect(container.textContent).not.toContain('Disable');
  });

  it('ignores a stale enable result after navigating away and back', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);
    const firstOriginResult = deferred<boolean>();
    mocks.pushNotifications.ensureRegistered
      .mockReturnValueOnce(firstOriginResult.promise)
      .mockResolvedValueOnce(false);

    const { container } = render(NotificationsPage);
    await settle();
    buttonWithText(container, 'Enable').click();
    await settle();

    mocks.activeServerId = 'remote';
    userPreferences.notificationSound = 'pop';
    await settle();
    mocks.activeServerId = 'origin';
    userPreferences.notificationSound = 'ding';
    await settle();
    buttonWithText(container, 'Enable').click();
    await settle();
    expect(container.textContent).toContain('Failed to enable push notifications');

    firstOriginResult.resolve(true);
    await settle();

    expect(container.textContent).toContain('Failed to enable push notifications');
    expect(container.textContent).not.toContain('Push notifications enabled');
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

    expect(mocks.pushNotifications.sendTestNotification).toHaveBeenCalledWith('origin');
    expect(container.textContent).toContain('Test notification sent.');
  });

  it('ignores a stale test result after navigating away and back', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.getPermission.mockReturnValue('granted');
    mocks.pushNotifications.isSubscribed.mockResolvedValue(true);
    const firstOriginResult = deferred<boolean>();
    mocks.pushNotifications.sendTestNotification
      .mockReturnValueOnce(firstOriginResult.promise)
      .mockResolvedValueOnce(false);

    const { container } = render(NotificationsPage);
    await settle();
    buttonWithText(container, 'Send test notification').click();
    await settle();

    mocks.activeServerId = 'remote';
    userPreferences.notificationSound = 'pop';
    await settle();
    mocks.activeServerId = 'origin';
    userPreferences.notificationSound = 'ding';
    await settle();
    buttonWithText(container, 'Send test notification').click();
    await settle();
    expect(container.textContent).toContain('Could not send a test notification');

    firstOriginResult.resolve(true);
    await settle();

    expect(container.textContent).toContain('Could not send a test notification');
    expect(container.textContent).not.toContain('Test notification sent.');
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
