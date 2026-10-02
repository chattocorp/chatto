import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { ConnectError, Code } from '@connectrpc/connect';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import PushDeviceSettings from './PushDeviceSettings.svelte';

const mocks = vi.hoisted(() => ({
  capability: 'supported' as 'supported' | 'ios_home_screen_required' | 'unsupported',
  permission: 'granted' as NotificationPermission | null,
  registered: true,
  failed: false,
  webPushRuntime: true,
  refreshPushSubscriptions: vi.fn(),
  sendTestNotification: vi.fn(),
  hasSavedPushRegistration: vi.fn()
}));

vi.mock('$lib/notifications/pushNotifications', () => ({
  getPermission: () => mocks.permission,
  getPushCapability: () => mocks.capability,
  hasFailedPushRegistration: () => mocks.failed,
  hasSavedPushRegistration: mocks.hasSavedPushRegistration,
  isBrowserWebPushRuntime: () => mocks.webPushRuntime,
  refreshPushSubscriptions: mocks.refreshPushSubscriptions,
  sendTestNotification: mocks.sendTestNotification
}));

vi.mock('$lib/notifications/pushDevice', () => ({
  describePushDevice: () => ({ browser: 'Firefox', platform: 'macOS' })
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

function renderSettings(pushNotificationsEnabled = true) {
  createTestServerScope({ serverId: 'origin', serverInfo: { pushNotificationsEnabled } });
  return render(PushDeviceSettings);
}

describe('PushDeviceSettings', () => {
  beforeEach(() => {
    mocks.capability = 'supported';
    mocks.permission = 'granted';
    mocks.webPushRuntime = true;
    mocks.hasSavedPushRegistration.mockReset();
    mocks.hasSavedPushRegistration.mockImplementation(() => mocks.registered);
    mocks.registered = true;
    mocks.failed = false;
    mocks.refreshPushSubscriptions.mockReset();
    mocks.refreshPushSubscriptions.mockResolvedValue(undefined);
    mocks.sendTestNotification.mockReset();
    mocks.sendTestNotification.mockResolvedValue(true);
  });

  it('names the device and origin that receive this server’s push notifications', async () => {
    const screen = renderSettings();

    await expect
      .element(screen.getByText('This server sends push notifications to this device.'))
      .toBeVisible();
    await expect.element(screen.getByText('Firefox on macOS')).toBeVisible();
    await expect
      .element(screen.getByText(`Notifications open Chatto at ${window.location.host}`))
      .toBeVisible();
    expect(mocks.hasSavedPushRegistration).toHaveBeenCalledWith('origin', 'viewer-1');
  });

  it('sends a test notification through this server', async () => {
    const screen = renderSettings();

    await screen.getByRole('button', { name: 'Send test notification' }).click();

    expect(mocks.sendTestNotification).toHaveBeenCalledWith('origin');
    await expect.element(screen.getByRole('status')).toHaveTextContent('Test notification sent.');
  });

  it('reports a failed delivery as a failed test, not as a network error', async () => {
    mocks.sendTestNotification.mockRejectedValue(
      new ConnectError('push notification could not be delivered', Code.Unavailable)
    );
    const screen = renderSettings();

    await screen.getByRole('button', { name: 'Send test notification' }).click();

    await expect
      .element(screen.getByRole('alert'))
      .toHaveTextContent('Could not send a test notification. Try again in a moment.');
  });

  it('asks the user to wait when tests are sent too often', async () => {
    mocks.sendTestNotification.mockRejectedValue(
      new ConnectError('test push notification rate limit exceeded', Code.ResourceExhausted)
    );
    const screen = renderSettings();

    await screen.getByRole('button', { name: 'Send test notification' }).click();

    await expect
      .element(screen.getByRole('alert'))
      .toHaveTextContent('Wait a few seconds before you send another test notification.');
  });

  it('explains a failed setup and lets the user try again', async () => {
    mocks.registered = false;
    mocks.failed = true;
    const screen = renderSettings();

    await expect
      .element(screen.getByText('Push notifications could not be set up on this device'))
      .toBeVisible();
    await screen.getByRole('button', { name: 'Try Again' }).click();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledOnce();
  });

  it('shows setup progress until this page saves the subscription', async () => {
    mocks.registered = false;
    const screen = renderSettings();

    await expect
      .element(screen.getByRole('status'))
      .toHaveTextContent('Setting up push notifications for this device…');
    expect(screen.container.querySelector('button')).toBeNull();
  });

  it('points to the Notifications page while permission is unset, without asking', async () => {
    mocks.permission = 'default';
    const screen = renderSettings();

    await expect
      .element(screen.getByText('Push notifications are off on this device'))
      .toBeVisible();
    await expect
      .element(screen.getByRole('link', { name: 'Open Notifications' }))
      .toHaveAttribute('href', '/chat/notifications');
    expect(mocks.sendTestNotification).not.toHaveBeenCalled();
  });

  it('explains blocked permission', async () => {
    mocks.permission = 'denied';
    const screen = renderSettings();

    await expect.element(screen.getByText('Push notifications blocked')).toBeVisible();
  });

  it('shows Home Screen guidance on iOS browsers', async () => {
    mocks.capability = 'ios_home_screen_required';
    mocks.permission = null;
    const screen = renderSettings();

    await expect.element(screen.getByText('Add Chatto to your Home Screen')).toBeVisible();
  });

  it('renders nothing when the server has no push configuration', async () => {
    const screen = renderSettings(false);

    expect(screen.container.querySelector('[data-testid="push-notification-settings"]')).toBeNull();
  });

  it('renders nothing outside a browser web app', async () => {
    mocks.webPushRuntime = false;
    const screen = renderSettings();

    expect(screen.container.querySelector('[data-testid="push-notification-settings"]')).toBeNull();
  });
});
