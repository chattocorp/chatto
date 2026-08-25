import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  NotificationDeliveryMode,
  notificationPolicyScopeKey,
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
      policy: vi.fn(),
      isPending: vi.fn(() => false)
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

  it('retains a matched room group and cycles an inherited cell to Off', async () => {
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
    expect(button.ariaLabel).toContain('Override: Inherit');
    expect(button.ariaLabel).toContain('Effective: Push notification');
    expect(button.ariaLabel).toContain('Activate to set Off');
  });

  it('explains the three delivery modes in the legend', () => {
    const { container } = render(NotificationPolicySettings);

    const legend = container.querySelector('[aria-label="Notification delivery modes"]');
    expect(legend?.textContent).toContain('Off');
    expect(legend?.textContent).toContain('Notification');
    expect(legend?.textContent).toContain('Push notification');
    expect(legend?.querySelector('[class~="icon-[uil--bell-slash]"]')).not.toBeNull();
    expect(legend?.querySelector('[class~="icon-[uil--bell]"]')).not.toBeNull();
    expect(legend?.querySelector('[class~="icon-[uil--mobile-android]"]')).not.toBeNull();
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
});
