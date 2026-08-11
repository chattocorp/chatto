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
    store: { notifications: mocks.notifications }
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
        NotificationDeliveryIntensity.OFF
      );
      expect(select.value).toBe(String(NotificationDeliveryIntensity.ALERT));
      expect(container.textContent).toContain('save rejected');
    });
  });
});
