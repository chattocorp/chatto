import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  NotificationDeliveryMode,
  notificationPolicyScopeKey,
  type NotificationPolicyField,
  type NotificationPolicyScope,
  type ScopedNotificationPolicy
} from '$lib/api-client/notifications';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    matrix: {
      loading: false,
      error: null as string | null,
      errorKind: null as 'load' | 'save' | null,
      load: vi.fn(),
      update: vi.fn(),
      resetServerDefaults: vi.fn(),
      policy: vi.fn(),
      isPending: vi.fn((_scope: NotificationPolicyScope, _field: NotificationPolicyField) => false)
    }
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    store: {
      notifications: { notificationPolicies: mocks.matrix },
      serverInfo: { name: 'Test Server' },
      navigation: {
        roomGroups: [{ id: 'group-1', name: 'Channels', roomIds: ['room-1', 'room-2'] }],
        rooms: [
          { id: 'room-1', name: 'general', viewerIsMember: true, type: 1 },
          { id: 'room-2', name: 'private', viewerIsMember: false, type: 1 },
          { id: 'dm-1', name: 'Taylor', viewerIsMember: true, type: 2 }
        ]
      }
    }
  })
}));

import NotificationPolicySettings from './NotificationPolicySettings.svelte';

function policy(scope: NotificationPolicyScope): ScopedNotificationPolicy {
  return {
    scope,
    overrides: {
      directMessages: null,
      directMentions: null,
      replies: null,
      roleMentions: null,
      hereMentions: null,
      allMentions: null,
      followedThreads: null,
      followedRooms: null,
      reactions: null
    },
    effective: {
      directMessages: NotificationDeliveryMode.PUSH_NOTIFICATION,
      directMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
      replies: NotificationDeliveryMode.PUSH_NOTIFICATION,
      roleMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
      hereMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
      allMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
      followedThreads: NotificationDeliveryMode.IN_APP_NOTIFICATION,
      followedRooms: NotificationDeliveryMode.OFF,
      reactions: NotificationDeliveryMode.IN_APP_NOTIFICATION
    }
  };
}

describe('NotificationPolicySettings', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.matrix.loading = false;
    mocks.matrix.error = null;
    mocks.matrix.errorKind = null;
    mocks.matrix.load.mockResolvedValue(undefined);
    mocks.matrix.update.mockResolvedValue(undefined);
    mocks.matrix.resetServerDefaults.mockResolvedValue(undefined);
    mocks.matrix.isPending.mockReturnValue(false);
    mocks.matrix.policy.mockImplementation((scope: NotificationPolicyScope) => policy(scope));
  });

  it('renders all causes and orders member-visible scopes by inheritance context', async () => {
    const { container } = render(NotificationPolicySettings);

    await vi.waitFor(() => expect(mocks.matrix.load).toHaveBeenCalled());
    expect(
      [...container.querySelectorAll('th[data-notification-scope]')].map((cell) =>
        cell.getAttribute('data-notification-scope')
      )
    ).toEqual(['server', 'roomGroup:group-1', 'room:room-1', 'room:dm-1']);
    expect(container.querySelector('th[data-notification-scope="room:room-2"]')).toBeNull();
    expect(container.querySelectorAll('[data-matrix-row]')).toHaveLength(9 * 4);
    expect(container.textContent).not.toContain('Room invitations');
  });

  it('retains a matched room group and cycles a server default to Off', async () => {
    const { container } = render(NotificationPolicySettings);
    const input = container.querySelector(
      '[data-testid="notification-scope-filter"] input, input[data-testid="notification-scope-filter"]'
    ) as HTMLInputElement;
    expect(input).not.toBeNull();
    input.value = 'general';
    input.dispatchEvent(new Event('input', { bubbles: true }));

    await vi.waitFor(() => {
      expect(
        [...container.querySelectorAll('th[data-notification-scope]')].map((cell) =>
          cell.getAttribute('data-notification-scope')
        )
      ).toEqual(['server', 'roomGroup:group-1', 'room:room-1']);
    });

    const button = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
    ) as HTMLButtonElement;
    button.click();
    expect(mocks.matrix.update).toHaveBeenCalledWith(
      { kind: 'server' },
      'directMessages',
      NotificationDeliveryMode.OFF
    );
    expect(button.ariaLabel).toContain('Default: Push notification');
    expect(button.ariaLabel).toContain('Activate to set Off');
    expect(button.querySelector('[class~="icon-[uil--link]"]')).toBeNull();
  });

  it('resets configured server cells to product defaults', async () => {
    mocks.matrix.policy.mockImplementation((scope: NotificationPolicyScope) => {
      const result = policy(scope);
      if (scope.kind === 'server') {
        result.overrides.directMessages = NotificationDeliveryMode.OFF;
        result.effective.directMessages = NotificationDeliveryMode.OFF;
      }
      return result;
    });
    const { container } = render(NotificationPolicySettings);

    const reset = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Reset to defaults')
    ) as HTMLButtonElement;
    expect(reset).toBeDefined();
    expect(reset.disabled).toBe(false);
    reset.click();
    expect(mocks.matrix.resetServerDefaults).toHaveBeenCalledOnce();
  });

  it('explains the three delivery modes in the legend', () => {
    const { container } = render(NotificationPolicySettings);

    const legend = container.querySelector('[aria-label="Notification delivery modes"]');
    expect(legend?.textContent).toContain('Off');
    expect(legend?.textContent).toContain('Notification');
    expect(legend?.textContent).toContain('Push notification');
    expect(legend?.querySelector('[class~="icon-[ph--bell-slash-fill]"]')).not.toBeNull();
    expect(legend?.querySelector('[class~="icon-[ph--bell-fill]"]')).not.toBeNull();
    expect(legend?.querySelector('[class~="icon-[ph--phone-fill]"]')).not.toBeNull();
    expect(legend?.querySelector('[class~="icon-[uil--link]"]')).not.toBeNull();
  });

  it('uses stable keys for every loaded scope', async () => {
    render(NotificationPolicySettings);
    await vi.waitFor(() => expect(mocks.matrix.load).toHaveBeenCalled());
    const loaded = mocks.matrix.load.mock.calls.at(-1)?.[0] as NotificationPolicyScope[];
    expect(loaded.map(notificationPolicyScopeKey)).toEqual([
      'server',
      'roomGroup:group-1',
      'room:room-1',
      'room:dm-1'
    ]);
  });

  it('renders per-cell loading placeholders while the visible scopes load', () => {
    mocks.matrix.loading = true;
    mocks.matrix.policy.mockReturnValue(undefined);

    const { container } = render(NotificationPolicySettings);

    expect(container.querySelectorAll('[class~="icon-[uil--spinner]"]')).toHaveLength(9 * 4);
    expect(container.querySelectorAll('td[data-notification-field] button')).toHaveLength(0);
    const placeholders = container.querySelectorAll(
      'td[data-notification-field] > span[role="status"]'
    );
    expect(placeholders).toHaveLength(9 * 4);
    expect([...placeholders].every((item) => item.textContent?.trim() === 'Loading...')).toBe(true);
  });

  it('keeps independent cells available while one update is pending', () => {
    mocks.matrix.isPending.mockImplementation(
      (scope: NotificationPolicyScope, field: NotificationPolicyField) =>
        scope.kind === 'server' && field === 'directMessages'
    );
    const { container } = render(NotificationPolicySettings);
    const pending = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
    ) as HTMLButtonElement;
    const available = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMentions"] button'
    ) as HTMLButtonElement;

    expect(pending.disabled).toBe(false);
    expect(pending.getAttribute('aria-disabled')).toBe('true');
    expect(pending.querySelector('[class~="icon-[uil--spinner]"]')).not.toBeNull();
    pending.click();
    expect(mocks.matrix.update).not.toHaveBeenCalled();

    expect(available.getAttribute('aria-disabled')).toBeNull();
    available.click();
    expect(mocks.matrix.update).toHaveBeenCalledWith(
      { kind: 'server' },
      'directMentions',
      NotificationDeliveryMode.OFF
    );
  });

  it('shows localized load and save errors without hiding the matrix', () => {
    mocks.matrix.error = 'Policy service unavailable';
    mocks.matrix.errorKind = 'load';
    mocks.matrix.policy.mockReturnValue(undefined);
    const loadFailure = render(NotificationPolicySettings);

    expect(loadFailure.container.textContent).toContain(
      'Failed to load notification policy: Policy service unavailable'
    );
    expect(loadFailure.container.querySelectorAll('[data-matrix-row]')).toHaveLength(9 * 4);
    loadFailure.unmount();

    mocks.matrix.error = 'Update was rejected';
    mocks.matrix.errorKind = 'save';
    mocks.matrix.policy.mockImplementation((scope: NotificationPolicyScope) => policy(scope));
    const saveFailure = render(NotificationPolicySettings);

    expect(saveFailure.container.textContent).toContain(
      'Failed to save notification policy: Update was rejected'
    );
    expect(
      saveFailure.container.querySelectorAll('td[data-notification-field] button')
    ).toHaveLength(9 * 4);
  });
});
