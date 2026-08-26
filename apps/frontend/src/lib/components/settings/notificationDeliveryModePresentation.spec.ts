import { beforeEach, describe, expect, it } from 'vitest';
import { NotificationDeliveryMode } from '$lib/api-client/notifications';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  notificationDeliveryModeLabel,
  notificationDeliveryModePresentation
} from './notificationDeliveryModePresentation';

describe('notification delivery mode presentation', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('uses the neutral crossed-out bell language for Off', () => {
    expect(notificationDeliveryModePresentation(NotificationDeliveryMode.OFF)).toEqual({
      icon: 'icon-[ph--bell-slash-fill]',
      tone: 'neutral',
      legendClass: 'text-text'
    });
  });

  it('uses orange warning language for in-app notifications and push', () => {
    expect(
      notificationDeliveryModePresentation(NotificationDeliveryMode.IN_APP_NOTIFICATION)
    ).toEqual({
      icon: 'icon-[ph--bell-fill]',
      tone: 'warning',
      legendClass: 'text-warning'
    });
    expect(
      notificationDeliveryModePresentation(NotificationDeliveryMode.PUSH_NOTIFICATION)
    ).toEqual({
      icon: 'icon-[ph--phone-fill]',
      tone: 'warning',
      legendClass: 'text-warning'
    });
  });

  it('provides the shared localized labels, including the inheritance marker', () => {
    expect(notificationDeliveryModeLabel(NotificationDeliveryMode.OFF)).toBe('Off');
    expect(notificationDeliveryModeLabel(NotificationDeliveryMode.IN_APP_NOTIFICATION)).toBe(
      'Notification'
    );
    expect(notificationDeliveryModeLabel(NotificationDeliveryMode.PUSH_NOTIFICATION)).toBe(
      'Push notification'
    );
    expect(notificationDeliveryModeLabel(null)).toBe('Inherit');
  });

  it('rejects unspecified and unknown delivery modes', () => {
    expect(() =>
      notificationDeliveryModePresentation(NotificationDeliveryMode.UNSPECIFIED)
    ).toThrow('Unsupported notification delivery mode');
    expect(() => notificationDeliveryModeLabel(NotificationDeliveryMode.UNSPECIFIED)).toThrow(
      'Unsupported notification delivery mode'
    );
  });
});
