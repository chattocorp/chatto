import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { ConnectError, Code } from '@connectrpc/connect';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { isPushPromptSnoozed } from '$lib/notifications/pushPrompt';
import PushDeviceSettings from './PushDeviceSettings.svelte';

const mocks = vi.hoisted(() => ({
  capability: 'supported' as 'supported' | 'ios_home_screen_required' | 'unsupported',
  permission: 'granted' as NotificationPermission | null,
  registered: true,
  failure: null as string | null,
  webPushRuntime: true,
  retryPushRegistration: vi.fn(),
  enablePushOnAllServers: vi.fn(),
  disablePushOnAllServers: vi.fn(),
  disabledOnDevice: false,
  sendTestNotification: vi.fn(),
  hasSavedPushRegistration: vi.fn()
}));

vi.mock('$lib/notifications/pushNotifications', () => ({
  enablePushOnAllServers: mocks.enablePushOnAllServers,
  disablePushOnAllServers: mocks.disablePushOnAllServers,
  isPushDisabledOnThisDevice: () => mocks.disabledOnDevice,
  getPermission: () => mocks.permission,
  getPushCapability: () => mocks.capability,
  hasSavedPushRegistration: mocks.hasSavedPushRegistration,
  isBrowserWebPushRuntime: () => mocks.webPushRuntime,
  pushRegistrationFailure: () => mocks.failure,
  retryPushRegistration: mocks.retryPushRegistration,
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
    mocks.failure = null;
    mocks.retryPushRegistration.mockReset();
    mocks.retryPushRegistration.mockResolvedValue(true);
    mocks.sendTestNotification.mockReset();
    mocks.sendTestNotification.mockResolvedValue(true);
    mocks.enablePushOnAllServers.mockReset();
    mocks.enablePushOnAllServers.mockResolvedValue({ permission: 'granted', registrations: [] });
    mocks.disablePushOnAllServers.mockReset();
    mocks.disablePushOnAllServers.mockResolvedValue(undefined);
    mocks.disabledOnDevice = false;
    window.localStorage.removeItem('chatto:pushPromptSnoozedUntil');
  });

  it('shows that push is on for every server on this device', async () => {
    const screen = renderSettings();
    const checkbox = screen.getByRole('checkbox', { name: 'Push notifications on this device' });

    await expect.element(checkbox).toBeChecked();
    await expect
      .element(screen.getByText('Firefox on macOS · Applies to every server on this device.'))
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
    mocks.failure = 'AbortError: Registration failed - push service error';
    const screen = renderSettings();

    await expect
      .element(screen.getByRole('alert'))
      .toHaveTextContent(
        'Push notifications could not be set up: AbortError: Registration failed - push service error'
      );
    await screen.getByRole('button', { name: 'Try Again' }).click();

    expect(mocks.retryPushRegistration).toHaveBeenCalledWith('origin');
  });

  it('shows setup progress until this page saves the subscription', async () => {
    mocks.registered = false;
    const screen = renderSettings();

    await expect
      .element(screen.getByText('Setting up push notifications for this device…'))
      .toBeVisible();
    await expect
      .element(screen.getByRole('button', { name: 'Send test notification' }))
      .not.toBeInTheDocument();
  });

  it('turns push on from the checkbox while permission is unset', async () => {
    mocks.permission = 'default';
    const screen = renderSettings();
    const checkbox = screen.getByRole('checkbox', { name: 'Push notifications on this device' });

    await expect.element(checkbox).not.toBeChecked();
    await checkbox.click();

    expect(mocks.enablePushOnAllServers).toHaveBeenCalledOnce();
    expect(isPushPromptSnoozed()).toBe(false);
  });

  it('explains a permission request that ended without a decision', async () => {
    mocks.permission = 'default';
    mocks.enablePushOnAllServers.mockResolvedValue({ permission: 'default', registrations: [] });
    const screen = renderSettings();
    const checkbox = screen.getByRole('checkbox', { name: 'Push notifications on this device' });

    await checkbox.click();

    await expect
      .element(screen.getByRole('alert'))
      .toHaveTextContent(/Notifications are not allowed for this site/);
    await expect.element(checkbox).not.toBeChecked();
    // A dismissed browser prompt counts as Not now for the invitation too.
    expect(isPushPromptSnoozed()).toBe(true);
    window.localStorage.removeItem('chatto:pushPromptSnoozedUntil');
  });

  it('turns push off for every server on this device', async () => {
    const screen = renderSettings();

    await screen.getByRole('checkbox', { name: 'Push notifications on this device' }).click();

    expect(mocks.disablePushOnAllServers).toHaveBeenCalledOnce();
  });

  it('turns push on again after the user turned it off', async () => {
    mocks.disabledOnDevice = true;
    const screen = renderSettings();
    const checkbox = screen.getByRole('checkbox', { name: 'Push notifications on this device' });

    await expect.element(checkbox).not.toBeChecked();
    await expect
      .element(screen.getByRole('button', { name: 'Send test notification' }))
      .not.toBeInTheDocument();
    await checkbox.click();

    expect(mocks.enablePushOnAllServers).toHaveBeenCalledOnce();
  });

  it('offers a retry when push could not be turned off for every server', async () => {
    mocks.disablePushOnAllServers.mockRejectedValue(new Error('server unreachable'));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const screen = renderSettings();

    await screen.getByRole('checkbox', { name: 'Push notifications on this device' }).click();

    await expect
      .element(screen.getByRole('alert'))
      .toHaveTextContent('Push notifications could not be turned off for every server.');
    await screen.getByRole('button', { name: 'Try Again' }).click();
    expect(mocks.disablePushOnAllServers).toHaveBeenCalledTimes(2);
    error.mockRestore();
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
