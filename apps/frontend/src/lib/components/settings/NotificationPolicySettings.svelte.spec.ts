import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { NotificationDeliveryIntensity, NotificationReason } from '$lib/api-client/notifications';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    notifications: {
      getPolicy: vi.fn(),
      setPolicyPreference: vi.fn()
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

describe('NotificationPolicySettings', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.notifications.getPolicy.mockResolvedValue([
      {
        reason: NotificationReason.DIRECT_MESSAGE,
        serverIntensity: NotificationDeliveryIntensity.ALERT,
        roomIntensity: NotificationDeliveryIntensity.UNSPECIFIED,
        effectiveIntensity: NotificationDeliveryIntensity.ALERT
      }
    ]);
    mocks.notifications.setPolicyPreference.mockRejectedValue(new Error('save rejected'));
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
    expect(select.value).toBe(String(NotificationDeliveryIntensity.ALERT));
    select.value = String(NotificationDeliveryIntensity.OFF);
    select.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => {
      expect(mocks.notifications.setPolicyPreference).toHaveBeenCalledWith(
        NotificationReason.DIRECT_MESSAGE,
        NotificationDeliveryIntensity.OFF,
        undefined
      );
      expect(select.value).toBe(String(NotificationDeliveryIntensity.ALERT));
      expect(container.textContent).toContain('save rejected');
    });
  });

  it('loads, changes, and clears policy at room scope', async () => {
    mocks.notifications.getPolicy.mockImplementation((roomId?: string) =>
      Promise.resolve([
        {
          reason: NotificationReason.DIRECT_MESSAGE,
          serverIntensity: NotificationDeliveryIntensity.ALERT,
          roomIntensity: roomId
            ? NotificationDeliveryIntensity.BADGE
            : NotificationDeliveryIntensity.UNSPECIFIED,
          effectiveIntensity: roomId
            ? NotificationDeliveryIntensity.BADGE
            : NotificationDeliveryIntensity.ALERT
        }
      ])
    );
    mocks.notifications.setPolicyPreference.mockResolvedValue([]);
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
    expect(directMessages.value).toBe(String(NotificationDeliveryIntensity.BADGE));
    directMessages.value = String(NotificationDeliveryIntensity.UNSPECIFIED);
    directMessages.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => {
      expect(mocks.notifications.setPolicyPreference).toHaveBeenCalledWith(
        NotificationReason.DIRECT_MESSAGE,
        NotificationDeliveryIntensity.UNSPECIFIED,
        'room-1'
      );
    });
  });
});
