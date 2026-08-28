import { NotificationDeliveryMode } from '$lib/api-client/notifications';
import type { MatrixCellTone } from '$lib/ui/matrix';
import { m } from '$lib/i18n/messages';

export type NotificationDeliveryModePresentation = {
  icon: string;
  tone: MatrixCellTone;
  legendClass: string;
};

/** Shared icon and tone language for notification delivery modes. */
export function notificationDeliveryModePresentation(
  mode: NotificationDeliveryMode
): NotificationDeliveryModePresentation {
  if (mode === NotificationDeliveryMode.OFF) {
    return {
      icon: 'icon-[ph--bell-slash-fill]',
      tone: 'neutral',
      legendClass: 'text-text'
    };
  }
  if (mode === NotificationDeliveryMode.UNREAD_BADGE) {
    return {
      icon: 'icon-[ph--bell-fill]',
      tone: 'neutral',
      legendClass: 'text-text'
    };
  }
  if (mode === NotificationDeliveryMode.IN_APP_NOTIFICATION) {
    return {
      icon: 'icon-[ph--bell-fill]',
      tone: 'warning',
      legendClass: 'text-warning'
    };
  }
  if (mode === NotificationDeliveryMode.PUSH_NOTIFICATION) {
    return {
      icon: 'icon-[ph--phone-fill]',
      tone: 'warning',
      legendClass: 'text-warning'
    };
  }
  throw new Error(`Unsupported notification delivery mode: ${mode}`);
}

/** Localized name used by cells, legends, and accessible descriptions. */
export function notificationDeliveryModeLabel(mode: NotificationDeliveryMode | null): string {
  if (mode === null) {
    return m('settings.notifications.policy.delivery_mode.inherit');
  }
  if (mode === NotificationDeliveryMode.OFF) {
    return m('settings.notifications.policy.delivery_mode.off');
  }
  if (mode === NotificationDeliveryMode.UNREAD_BADGE) {
    return m('settings.notifications.policy.delivery_mode.badge');
  }
  if (mode === NotificationDeliveryMode.IN_APP_NOTIFICATION) {
    return m('settings.notifications.policy.delivery_mode.notification');
  }
  if (mode === NotificationDeliveryMode.PUSH_NOTIFICATION) {
    return m('settings.notifications.policy.delivery_mode.push_notification');
  }
  throw new Error(`Unsupported notification delivery mode: ${mode}`);
}
