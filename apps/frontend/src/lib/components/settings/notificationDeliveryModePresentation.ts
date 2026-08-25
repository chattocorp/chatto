import { NotificationDeliveryMode } from '$lib/api-client/notifications';
import type { MatrixCellTone } from '$lib/components/matrix';
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
      icon: 'icon-[uil--bell-slash]',
      tone: 'neutral',
      legendClass: 'bg-surface-emphasized text-muted'
    };
  }
  if (mode === NotificationDeliveryMode.IN_APP_NOTIFICATION) {
    return {
      icon: 'icon-[uil--bell]',
      tone: 'warning',
      legendClass: 'bg-warning/20 text-warning'
    };
  }
  if (mode === NotificationDeliveryMode.PUSH_NOTIFICATION) {
    return {
      icon: 'icon-[uil--mobile-android]',
      tone: 'warning',
      legendClass: 'bg-warning/20 text-warning'
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
  if (mode === NotificationDeliveryMode.IN_APP_NOTIFICATION) {
    return m('settings.notifications.policy.delivery_mode.notification');
  }
  if (mode === NotificationDeliveryMode.PUSH_NOTIFICATION) {
    return m('settings.notifications.policy.delivery_mode.push_notification');
  }
  throw new Error(`Unsupported notification delivery mode: ${mode}`);
}
