<!--
@component

Notification-specific adapter for the shared matrix cell shell. It owns the
scope-specific override cycle. Server defaults render as concrete values.
Inherited room-group and room cells render the effective mode at reduced
intensity.
-->
<script lang="ts">
  import { MatrixCellButton } from '$lib/ui/matrix';
  import {
    NotificationDeliveryMode,
    type NotificationPolicyField,
    type NotificationPolicyScope
  } from '$lib/api-client/notifications';
  import { m } from '$lib/i18n/messages';
  import {
    notificationDeliveryModeLabel,
    notificationDeliveryModePresentation
  } from './notificationDeliveryModePresentation';

  let {
    field,
    causeLabel,
    scope,
    scopeLabel,
    override,
    effective,
    loading = false,
    disabled = false,
    onChange
  }: {
    field: NotificationPolicyField;
    causeLabel: string;
    scope: NotificationPolicyScope;
    scopeLabel: string;
    override: NotificationDeliveryMode | null;
    effective: NotificationDeliveryMode;
    loading?: boolean;
    disabled?: boolean;
    onChange: (next: NotificationDeliveryMode | null) => void;
  } = $props();

  const canInherit = $derived(scope.kind !== 'server');
  const visual = $derived(override ?? effective);
  const next = $derived(nextMode(override, effective, canInherit));
  const explicitVisual = $derived(!canInherit || override !== null);
  const usesDefault = $derived(!canInherit && override === null);
  const presentation = $derived(notificationDeliveryModePresentation(visual));
  const ariaLabel = $derived(
    usesDefault
      ? m('settings.notifications.policy.server_default_cell_aria', {
          cause: causeLabel,
          scope: scopeLabel,
          current: notificationDeliveryModeLabel(effective),
          next: notificationDeliveryModeLabel(next)
        })
      : m('settings.notifications.policy.cell_aria', {
          cause: causeLabel,
          scope: scopeLabel,
          override:
            override === null
              ? notificationDeliveryModeLabel(null)
              : notificationDeliveryModeLabel(override),
          effective: notificationDeliveryModeLabel(effective),
          next: notificationDeliveryModeLabel(next)
        })
  );

  function nextMode(
    current: NotificationDeliveryMode | null,
    currentEffective: NotificationDeliveryMode,
    inheritanceAvailable: boolean
  ): NotificationDeliveryMode | null {
    if (current === null && inheritanceAvailable) return NotificationDeliveryMode.OFF;
    const displayed = current ?? currentEffective;
    if (displayed === NotificationDeliveryMode.OFF) {
      return NotificationDeliveryMode.UNREAD_BADGE;
    }
    if (displayed === NotificationDeliveryMode.UNREAD_BADGE) {
      return NotificationDeliveryMode.IN_APP_NOTIFICATION;
    }
    if (displayed === NotificationDeliveryMode.IN_APP_NOTIFICATION) {
      return NotificationDeliveryMode.PUSH_NOTIFICATION;
    }
    return inheritanceAvailable ? null : NotificationDeliveryMode.OFF;
  }
</script>

<span
  data-notification-field={field}
  data-notification-scope={scope.kind}
  data-notification-source={usesDefault ? 'default' : override === null ? 'inherited' : 'override'}
>
  <MatrixCellButton
    tone={presentation.tone}
    explicit={explicitVisual}
    variant="icon"
    icon={presentation.icon}
    {loading}
    {disabled}
    pressed={override !== null}
    {ariaLabel}
    onActivate={() => onChange(next)}
  />
</span>
