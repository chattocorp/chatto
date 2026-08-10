<script lang="ts">
  import { FormSection, Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    NotificationDeliveryIntensity,
    NotificationReason,
    type NotificationPolicyItem
  } from '$lib/api-client/notifications';

  const serverScope = useServerScope();
  const notificationStore = $derived(serverScope.store.notifications);
  let preferences = $state.raw<NotificationPolicyItem[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let savingReason = $state<NotificationReason | null>(null);

  const reasons = [
    NotificationReason.DIRECT_MESSAGE,
    NotificationReason.DIRECT_MENTION,
    NotificationReason.REPLY,
    NotificationReason.ROLE_MENTION,
    NotificationReason.HERE,
    NotificationReason.ALL,
    NotificationReason.FOLLOWED_THREAD,
    NotificationReason.FOLLOWED_ROOM,
    NotificationReason.REACTION
  ];

  $effect(() => {
    void load();
  });

  async function load() {
    loading = true;
    error = null;
    if (!notificationStore?.getPolicy) {
      loading = false;
      return;
    }
    try {
      preferences = await notificationStore.getPolicy();
    } catch (cause) {
      error =
        cause instanceof Error ? cause.message : m('settings.notifications.policy.load_failed');
    } finally {
      loading = false;
    }
  }

  async function change(reason: NotificationReason, event: Event) {
    const intensity = Number(
      (event.currentTarget as HTMLSelectElement).value
    ) as NotificationDeliveryIntensity;
    savingReason = reason;
    error = null;
    try {
      preferences = await notificationStore.setPolicyPreference(reason, intensity);
    } catch (cause) {
      error =
        cause instanceof Error ? cause.message : m('settings.notifications.policy.save_failed');
    } finally {
      savingReason = null;
    }
  }

  function reasonLabel(reason: NotificationReason): string {
    switch (reason) {
      case NotificationReason.DIRECT_MESSAGE:
        return m('settings.notifications.policy.reason.direct_message');
      case NotificationReason.DIRECT_MENTION:
        return m('settings.notifications.policy.reason.direct_mention');
      case NotificationReason.REPLY:
        return m('settings.notifications.policy.reason.reply');
      case NotificationReason.ROLE_MENTION:
        return m('settings.notifications.policy.reason.role_mention');
      case NotificationReason.HERE:
        return m('settings.notifications.policy.reason.here');
      case NotificationReason.ALL:
        return m('settings.notifications.policy.reason.all');
      case NotificationReason.FOLLOWED_THREAD:
        return m('settings.notifications.policy.reason.followed_thread');
      case NotificationReason.FOLLOWED_ROOM:
        return m('settings.notifications.policy.reason.followed_room');
      case NotificationReason.REACTION:
        return m('settings.notifications.policy.reason.reaction');
      default:
        return m('settings.notifications.policy.reason.activity');
    }
  }

  function intensityLabel(intensity: NotificationDeliveryIntensity): string {
    switch (intensity) {
      case NotificationDeliveryIntensity.OFF:
        return m('settings.notifications.policy.intensity.off');
      case NotificationDeliveryIntensity.BADGE:
        return m('settings.notifications.policy.intensity.badge');
      case NotificationDeliveryIntensity.ALERT:
        return m('settings.notifications.policy.intensity.alert');
      default:
        return m('settings.notifications.policy.intensity.inherit');
    }
  }
</script>

<FormSection title={m('settings.notifications.policy.title')} maxWidth="max-w-2xl" bordered>
  <p class="mb-3 text-sm text-muted">{m('settings.notifications.policy.description')}</p>
  {#if error}<Hint tone="danger">{error}</Hint>{/if}
  {#if loading}
    <p class="py-3 text-sm text-muted">{m('common.loading')}</p>
  {:else}
    <div class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each reasons as reason (reason)}
        {@const preference = preferences.find((candidate) => candidate.reason === reason)}
        <label class="flex items-center justify-between gap-4 px-3 py-3">
          <span class="min-w-0">
            <span class="block font-medium">{reasonLabel(reason)}</span>
            <span class="block text-xs text-muted">
              {m('settings.notifications.policy.effective', {
                intensity: intensityLabel(
                  preference?.effectiveIntensity ?? NotificationDeliveryIntensity.OFF
                )
              })}
            </span>
          </span>
          <select
            class="input w-auto min-w-[120px] text-sm"
            aria-label={reasonLabel(reason)}
            value={String(preference?.serverIntensity ?? NotificationDeliveryIntensity.UNSPECIFIED)}
            disabled={savingReason !== null}
            onchange={(event) => change(reason, event)}
          >
            <option value={String(NotificationDeliveryIntensity.UNSPECIFIED)}
              >{intensityLabel(NotificationDeliveryIntensity.UNSPECIFIED)}</option
            >
            <option value={String(NotificationDeliveryIntensity.OFF)}
              >{intensityLabel(NotificationDeliveryIntensity.OFF)}</option
            >
            <option value={String(NotificationDeliveryIntensity.BADGE)}
              >{intensityLabel(NotificationDeliveryIntensity.BADGE)}</option
            >
            <option value={String(NotificationDeliveryIntensity.ALERT)}
              >{intensityLabel(NotificationDeliveryIntensity.ALERT)}</option
            >
          </select>
        </label>
      {/each}
    </div>
  {/if}
</FormSection>
