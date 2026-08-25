<!--
@component

Notification-specific adapter for the shared matrix cell shell. It owns the
scope-specific override cycle. Server defaults render as concrete values.
Inherited room-group and room cells render the effective mode with a link
marker so their source is not encoded by intensity alone.
-->
<script lang="ts">
  import { MatrixCellButton, type MatrixCellTone } from '$lib/components/matrix';
  import {
    NotificationDeliveryMode,
    type NotificationPolicyField,
    type NotificationPolicyScope
  } from '$lib/api-client/notifications';
  import { m } from '$lib/i18n/messages';

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
  const icon = $derived(modeIcon(visual));
  const tone = $derived(modeTone(visual));
  const ariaLabel = $derived(
    usesDefault
      ? m('settings.notifications.policy.server_default_cell_aria', {
          cause: causeLabel,
          scope: scopeLabel,
          current: modeLabel(effective),
          next: modeLabel(next)
        })
      : m('settings.notifications.policy.cell_aria', {
          cause: causeLabel,
          scope: scopeLabel,
          override: override === null ? modeLabel(null) : modeLabel(override),
          effective: modeLabel(effective),
          next: modeLabel(next)
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
      return NotificationDeliveryMode.IN_APP_NOTIFICATION;
    }
    if (displayed === NotificationDeliveryMode.IN_APP_NOTIFICATION) {
      return NotificationDeliveryMode.PUSH_NOTIFICATION;
    }
    return inheritanceAvailable ? null : NotificationDeliveryMode.OFF;
  }

  function modeLabel(mode: NotificationDeliveryMode | null): string {
    if (mode === NotificationDeliveryMode.OFF) {
      return m('settings.notifications.policy.delivery_mode.off');
    }
    if (mode === NotificationDeliveryMode.IN_APP_NOTIFICATION) {
      return m('settings.notifications.policy.delivery_mode.notification');
    }
    if (mode === NotificationDeliveryMode.PUSH_NOTIFICATION) {
      return m('settings.notifications.policy.delivery_mode.push_notification');
    }
    return m('settings.notifications.policy.delivery_mode.inherit');
  }

  function modeIcon(mode: NotificationDeliveryMode): string {
    if (mode === NotificationDeliveryMode.OFF) return 'icon-[uil--bell-slash]';
    if (mode === NotificationDeliveryMode.IN_APP_NOTIFICATION) return 'icon-[uil--bell]';
    return 'icon-[uil--mobile-android]';
  }

  function modeTone(mode: NotificationDeliveryMode): MatrixCellTone {
    if (mode === NotificationDeliveryMode.OFF) return 'neutral';
    return 'warning';
  }
</script>

<span
  data-notification-field={field}
  data-notification-scope={scope.kind}
  data-notification-source={usesDefault ? 'default' : override === null ? 'inherited' : 'override'}
>
  <MatrixCellButton
    {tone}
    explicit={explicitVisual}
    {icon}
    {loading}
    {disabled}
    inheritedMarker={canInherit && override === null}
    pressed={override !== null}
    {ariaLabel}
    title={ariaLabel}
    onActivate={() => onChange(next)}
  />
</span>
