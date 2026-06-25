import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PushNotificationPrompt from './PushNotificationPrompt.svelte';

const mocks = vi.hoisted(() => ({
  ensureRegistered: vi.fn(),
  getPermission: vi.fn(),
  isSupported: vi.fn(),
  toastSuccess: vi.fn(),
  toastWarning: vi.fn(),
  toastError: vi.fn(),
  serverInfo: {
    pushNotificationsEnabled: true,
    vapidPublicKey: 'vapid-key' as string | null
  }
}));

vi.mock('$lib/notifications/pushNotifications', () => ({
  ensureRegistered: mocks.ensureRegistered,
  getPermission: mocks.getPermission,
  isSupported: mocks.isSupported
}));

vi.mock('$lib/ui/toast', () => ({
  toast: {
    success: mocks.toastSuccess,
    warning: mocks.toastWarning,
    error: mocks.toastError
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    originServer: { id: 'origin' },
    getStore: () => ({
      serverInfo: mocks.serverInfo
    })
  }
}));

async function settle() {
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

describe('PushNotificationPrompt', () => {
  beforeEach(() => {
    localStorage.clear();
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
    mocks.ensureRegistered.mockReset();
    mocks.ensureRegistered.mockResolvedValue(true);
    mocks.getPermission.mockReset();
    mocks.getPermission.mockReturnValue('default');
    mocks.isSupported.mockReset();
    mocks.isSupported.mockReturnValue(true);
    mocks.toastSuccess.mockReset();
    mocks.toastWarning.mockReset();
    mocks.toastError.mockReset();
  });

  it('shows the prompt when push is configured, supported, and permission is unset', async () => {
    const { container } = render(PushNotificationPrompt, { props: { userId: 'user-1' } });
    await settle();

    expect(container.textContent).toContain('Enable push notifications');
    expect(container.textContent).toContain('DMs, mentions, and replies');
    await expect.element(buttonWithText(container, 'Enable')).toBeVisible();
    await expect.element(buttonWithText(container, 'No thanks')).toBeVisible();
  });

  it('does not show when permission is already granted', async () => {
    mocks.getPermission.mockReturnValue('granted');

    const { container } = render(PushNotificationPrompt, { props: { userId: 'user-1' } });
    await settle();

    expect(container.textContent).not.toContain('Enable push notifications');
  });

  it('persists opt-out for the current server and user', async () => {
    const { container } = render(PushNotificationPrompt, { props: { userId: 'user-1' } });
    await settle();

    buttonWithText(container, 'No thanks').click();
    await settle();

    expect(container.textContent).not.toContain('Enable push notifications');
    expect(localStorage.getItem('chatto:i:origin:user:user-1:pushPromptDismissed')).toBe('1');
  });

  it('does not show after the user opted out locally', async () => {
    localStorage.setItem('chatto:i:origin:user:user-1:pushPromptDismissed', '1');

    const { container } = render(PushNotificationPrompt, { props: { userId: 'user-1' } });
    await settle();

    expect(container.textContent).not.toContain('Enable push notifications');
  });

  it('enables push through the registration helper', async () => {
    mocks.ensureRegistered.mockImplementation(async () => {
      mocks.getPermission.mockReturnValue('granted');
      return true;
    });

    const { container } = render(PushNotificationPrompt, { props: { userId: 'user-1' } });
    await settle();

    buttonWithText(container, 'Enable').click();
    await settle();

    expect(mocks.ensureRegistered).toHaveBeenCalledWith('vapid-key', { prompt: true });
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Push notifications enabled');
    expect(container.textContent).not.toContain('Enable push notifications');
  });
});
