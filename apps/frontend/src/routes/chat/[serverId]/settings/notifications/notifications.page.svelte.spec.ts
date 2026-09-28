import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import NotificationsPage from './+page.svelte';

import { q } from '$lib/test-utils';
import {
  getServerNotificationPreferences,
  resetServerNotificationPreferencesForTests
} from '$lib/state/serverNotificationPreferences.svelte';
import { defaultNotificationSoundFilters } from '$lib/audio/notificationSounds';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  playNotificationSound: vi.fn(),
  notifications: {
    getPolicy: vi.fn().mockResolvedValue(null),
    updatePolicy: vi.fn().mockResolvedValue(null)
  },
  serverInfo: {
    name: 'Test Server',
    pushNotificationsEnabled: false,
    vapidPublicKey: null as string | null
  },
  pushNotifications: {
    enablePushOnAllServers: vi.fn(),
    isBrowserWebPushRuntime: vi.fn(),
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
  enablePushOnAllServers: mocks.pushNotifications.enablePushOnAllServers,
  isBrowserWebPushRuntime: mocks.pushNotifications.isBrowserWebPushRuntime,
  getPushCapability: mocks.pushNotifications.getPushCapability,
  getPermission: mocks.pushNotifications.getPermission,
  isSubscribed: mocks.pushNotifications.isSubscribed,
  sendTestNotification: mocks.pushNotifications.sendTestNotification
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: (serverId: string) => serverId === 'origin'
  }
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

const api = { batchGetNotificationPolicies: vi.fn().mockResolvedValue([]) };
let server: TestServerScope;

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
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
  const button = Array.from(container.querySelectorAll('button')).find(
    (candidate) => candidate.textContent?.trim() === text
  );
  if (!button) {
    throw new Error(`Button with text "${text}" not found`);
  }
  return button;
}

function notificationPreferences(serverId = server.serverId) {
  return getServerNotificationPreferences(serverId);
}

function notificationPreferencesStorageKey(serverId = server.serverId) {
  return `chatto:i:${serverId}:notificationPreferences`;
}

function createServerScope(serverId: string) {
  return createTestServerScope({
    serverId,
    api,
    store: {
      serverInfo: mocks.serverInfo,
      notifications: mocks.notifications,
      navigation: { roomGroups: [], rooms: [] }
    }
  });
}

describe('Notification settings page', () => {
  beforeEach(() => {
    queryClient.clear();
    localStorage.clear();
    localStorage.setItem(
      'chatto:preferences',
      JSON.stringify({
        notificationSound: 'chime-up',
        notificationSoundFilters: defaultNotificationSoundFilters
      })
    );
    resetServerNotificationPreferencesForTests();
    server = createServerScope('origin');
    mocks.playNotificationSound.mockClear();
    mocks.notifications.getPolicy.mockClear();
    mocks.notifications.getPolicy.mockResolvedValue(null);
    mocks.notifications.updatePolicy.mockClear();
    mocks.notifications.updatePolicy.mockResolvedValue(null);
    api.batchGetNotificationPolicies.mockClear();
    mocks.serverInfo.pushNotificationsEnabled = false;
    mocks.serverInfo.vapidPublicKey = null;
    mocks.pushNotifications.enablePushOnAllServers.mockReset();
    mocks.pushNotifications.enablePushOnAllServers.mockResolvedValue({
      permission: 'granted',
      registrations: [
        {
          serverId: 'origin',
          userId: 'origin-user',
          vapidPublicKey: 'vapid-key',
          registered: true
        }
      ]
    });
    mocks.pushNotifications.isBrowserWebPushRuntime.mockReset();
    mocks.pushNotifications.isBrowserWebPushRuntime.mockReturnValue(true);
    mocks.pushNotifications.getPermission.mockReset();
    mocks.pushNotifications.getPermission.mockReturnValue('default');
    mocks.pushNotifications.getPushCapability.mockReset();
    mocks.pushNotifications.getPushCapability.mockReturnValue('supported');
    mocks.pushNotifications.isSubscribed.mockReset();
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);
    mocks.pushNotifications.sendTestNotification.mockReset();
    mocks.pushNotifications.sendTestNotification.mockResolvedValue(true);
  });

  it('selects and persists a non-silent notification sound', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(3);
    const softPopButton = buttonWithText(container, 'Soft Pop');
    softPopButton.click();
    flushSync();

    expect(notificationPreferences().notificationSound).toBe('pop');
    expect(
      JSON.parse(localStorage.getItem(notificationPreferencesStorageKey()) ?? '{}')
    ).toMatchObject({
      notificationSound: 'pop'
    });
    expect(mocks.playNotificationSound).toHaveBeenCalledWith(
      'pop',
      defaultNotificationSoundFilters
    );
    await expect.element(softPopButton).toHaveClass(/choice-row-selected/);
  });

  it('keeps sound storage separate for each server', async () => {
    const origin = render(NotificationsPage);
    await settle();
    buttonWithText(origin.container, 'Soft Pop').click();
    flushSync();
    origin.unmount();

    // The server layout remounts the page with a new scope for another server.
    server = createServerScope('remote');
    const remote = render(NotificationsPage);
    await settle();
    buttonWithText(remote.container, 'Falling Chime').click();
    flushSync();

    expect(notificationPreferences('origin').notificationSound).toBe('pop');
    expect(notificationPreferences('remote').notificationSound).toBe('chime-down');
    expect(
      JSON.parse(localStorage.getItem(notificationPreferencesStorageKey('origin')) ?? '{}')
    ).toMatchObject({ notificationSound: 'pop' });
    expect(
      JSON.parse(localStorage.getItem(notificationPreferencesStorageKey('remote')) ?? '{}')
    ).toMatchObject({ notificationSound: 'chime-down' });
  });

  it('selects silent mode without previewing a sound', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const silentButton = buttonWithText(container, 'Silent');
    silentButton.click();
    flushSync();

    expect(notificationPreferences().notificationSound).toBe('silent');
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
    await expect.element(silentButton).toHaveClass(/choice-row-selected/);
  });

  it('shows the push enable path when configured and not subscribed', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);

    const { container } = render(NotificationsPage);
    await settle();

    const pushSettings = q(container, '[data-testid="push-notification-settings"]')!;
    const orderedSections = [
      ...container.querySelectorAll('[data-testid="push-notification-settings"], .panel-shell')
    ];
    expect(container.textContent).toContain('Push Notifications');
    expect(orderedSections[0]).toBe(pushSettings);
    expect(pushSettings.querySelector('button')?.textContent?.trim()).toBe('Enable');
    await expect.element(buttonWithText(container, 'Enable')).toBeVisible();
    expect(container.textContent).not.toContain('Disable');
  });

  it('offers an independent push subscription for remote servers', async () => {
    server = createServerScope('remote');
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
    server = createServerScope('remote');
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isBrowserWebPushRuntime.mockReturnValue(false);

    const { container } = render(NotificationsPage);
    await settle();

    expect(container.textContent).not.toContain('Push Notifications');
    expect(mocks.pushNotifications.isSubscribed).not.toHaveBeenCalled();
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
    expect(mocks.pushNotifications.enablePushOnAllServers).not.toHaveBeenCalled();
  });

  it('enables push notifications through the registration helper', async () => {
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.pushNotifications.isSubscribed.mockResolvedValue(false);
    mocks.pushNotifications.enablePushOnAllServers.mockImplementation(async () => {
      mocks.pushNotifications.getPermission.mockReturnValue('granted');
      return {
        permission: 'granted',
        registrations: [
          {
            serverId: 'origin',
            userId: 'origin-user',
            vapidPublicKey: 'vapid-key',
            registered: true
          }
        ]
      };
    });

    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'Enable').click();
    await settle();

    expect(mocks.pushNotifications.enablePushOnAllServers).toHaveBeenCalledOnce();
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

    expect(mocks.pushNotifications.sendTestNotification).toHaveBeenCalledWith('origin');
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

    expect(notificationPreferences().notificationSoundFilters).toEqual({
      volume: 1.5,
      highPassHz: 500,
      lowPassHz: 7904,
      echo: 35,
      reverb: 45,
      crunch: 55
    });
    expect(
      JSON.parse(localStorage.getItem(notificationPreferencesStorageKey()) ?? '{}')
    ).toMatchObject({
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

    expect(notificationPreferences().notificationSoundFilters).toEqual(
      defaultNotificationSoundFilters
    );
    expect(
      JSON.parse(localStorage.getItem(notificationPreferencesStorageKey()) ?? '{}')
    ).toMatchObject({
      notificationSoundFilters: defaultNotificationSoundFilters
    });
    expect(container.textContent).toContain('100%');
  });
});
// Title composition has separate coverage; these fixtures model notification preferences only.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
