import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  NotificationDeliveryMode,
  type NotificationPolicy,
  type NotificationPolicyOverrides
} from '$lib/api-client/notifications';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    notifications: {
      getPolicy: vi.fn(),
      updatePolicy: vi.fn()
    }
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    store: {
      notifications: mocks.notifications,
      serverInfo: { name: 'Test Server' },
      navigation: {
        rooms: [{ id: 'room-1', name: 'general', viewerIsMember: true }]
      }
    }
  })
}));

import NotificationPolicySettings from './NotificationPolicySettings.svelte';

function notificationPolicy(
  overrides: Partial<NotificationPolicyOverrides> = {},
  effectiveOverrides: Partial<NotificationPolicy['effective']> = {}
): NotificationPolicy {
  return {
    overrides: {
      directMessages: null,
      directMentions: null,
      replies: null,
      roleMentions: null,
      hereMentions: null,
      allMentions: null,
      followedThreads: null,
      followedRooms: null,
      reactions: null,
      ...overrides
    },
    effective: {
      directMessages: NotificationDeliveryMode.ALERT,
      directMentions: NotificationDeliveryMode.ALERT,
      replies: NotificationDeliveryMode.ALERT,
      roleMentions: NotificationDeliveryMode.ALERT,
      hereMentions: NotificationDeliveryMode.ALERT,
      allMentions: NotificationDeliveryMode.ALERT,
      followedThreads: NotificationDeliveryMode.SILENT,
      followedRooms: NotificationDeliveryMode.OFF,
      reactions: NotificationDeliveryMode.SILENT,
      ...effectiveOverrides
    }
  };
}

describe('NotificationPolicySettings', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.notifications.getPolicy.mockResolvedValue(
      notificationPolicy({ directMessages: NotificationDeliveryMode.ALERT })
    );
    mocks.notifications.updatePolicy.mockRejectedValue(new Error('save rejected'));
  });

  it('shows only implemented causes and restores a rejected selection', async () => {
    const { container } = render(NotificationPolicySettings);
    const select = await vi.waitFor(() => {
      const element = container.querySelector(
        'select[aria-label="Direct messages"]'
      ) as HTMLSelectElement | null;
      expect(element).not.toBeNull();
      return element!;
    });

    expect(container.querySelector('select[aria-label="Room invitations"]')).toBeNull();
    expect(select.value).toBe(String(NotificationDeliveryMode.ALERT));
    select.value = String(NotificationDeliveryMode.OFF);
    select.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => {
      expect(mocks.notifications.updatePolicy).toHaveBeenCalledWith(
        { directMessages: NotificationDeliveryMode.OFF },
        undefined
      );
      expect(select.value).toBe(String(NotificationDeliveryMode.ALERT));
      expect(container.textContent).toContain('save rejected');
    });
  });

  it('loads, changes, and clears policy at room scope', async () => {
    mocks.notifications.getPolicy.mockImplementation((roomId?: string) =>
      Promise.resolve(
        notificationPolicy(
          {
            directMessages: roomId
              ? NotificationDeliveryMode.SILENT
              : NotificationDeliveryMode.ALERT
          },
          {
            directMessages: roomId
              ? NotificationDeliveryMode.SILENT
              : NotificationDeliveryMode.ALERT
          }
        )
      )
    );
    mocks.notifications.updatePolicy.mockResolvedValue(notificationPolicy());
    const { container } = render(NotificationPolicySettings);
    const scope = await vi.waitFor(() => {
      const element = container.querySelector(
        'select[aria-label="Notification policy"]'
      ) as HTMLSelectElement | null;
      expect(element).not.toBeNull();
      return element!;
    });
    scope.value = 'room-1';
    scope.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => {
      expect(mocks.notifications.getPolicy).toHaveBeenCalledWith('room-1');
    });
    const directMessages = container.querySelector(
      'select[aria-label="Direct messages"]'
    ) as HTMLSelectElement;
    expect(directMessages.value).toBe(String(NotificationDeliveryMode.SILENT));
    directMessages.value = String(NotificationDeliveryMode.UNSPECIFIED);
    directMessages.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => {
      expect(mocks.notifications.updatePolicy).toHaveBeenCalledWith(
        { directMessages: null },
        'room-1'
      );
    });
  });
});
