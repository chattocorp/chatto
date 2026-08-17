<script lang="ts">
  import { FormSection, Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    NotificationDeliveryIntensity,
    NotificationPolicyKind,
    type NotificationPolicyItem
  } from '$lib/api-client/notifications';

  const serverScope = useServerScope();
  const notificationStore = $derived(serverScope.store.notifications);
  const policyRooms = $derived(
    (serverScope.store.navigation?.rooms ?? []).filter((room) => room.viewerIsMember)
  );
  let preferences = $state.raw<NotificationPolicyItem[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let savingReason = $state<NotificationPolicyKind | null>(null);
  let selectedRoomId = $state('');
  let loadGeneration = 0;

  const reasons = [
    NotificationPolicyKind.DIRECT_MESSAGE,
    NotificationPolicyKind.DIRECT_MENTION,
    NotificationPolicyKind.REPLY,
    NotificationPolicyKind.ROLE_MENTION,
    NotificationPolicyKind.HERE,
    NotificationPolicyKind.ALL,
    NotificationPolicyKind.FOLLOWED_THREAD,
    NotificationPolicyKind.FOLLOWED_ROOM,
    NotificationPolicyKind.REACTION
  ];

  $effect(() => {
    const roomId = selectedRoomId;
    void load(roomId);
  });

  async function load(roomId: string) {
    const generation = ++loadGeneration;
    loading = true;
    error = null;
    if (!notificationStore?.getPolicy) {
      loading = false;
      return;
    }
    try {
      const nextPreferences = await notificationStore.getPolicy(roomId || undefined);
      if (generation !== loadGeneration) return;
      preferences = nextPreferences;
    } catch (cause) {
      if (generation !== loadGeneration) return;
      error =
        cause instanceof Error ? cause.message : m('settings.notifications.policy.load_failed');
    } finally {
      if (generation === loadGeneration) loading = false;
    }
  }

  async function change(reason: NotificationPolicyKind, event: Event) {
    const select = event.currentTarget as HTMLSelectElement;
    const preference = preferences.find((candidate) => candidate.reason === reason);
    const roomId = selectedRoomId || undefined;
    const previousIntensity = roomId
      ? (preference?.roomIntensity ?? NotificationDeliveryIntensity.UNSPECIFIED)
      : (preference?.serverIntensity ?? NotificationDeliveryIntensity.UNSPECIFIED);
    const intensity = Number(select.value) as NotificationDeliveryIntensity;
    savingReason = reason;
    error = null;
    try {
      preferences = await notificationStore.setPolicyPreference(reason, intensity, roomId);
    } catch (cause) {
      select.value = String(previousIntensity);
      preferences = [...preferences];
      error =
        cause instanceof Error ? cause.message : m('settings.notifications.policy.save_failed');
    } finally {
      savingReason = null;
    }
  }

  function reasonLabel(reason: NotificationPolicyKind): string {
    switch (reason) {
      case NotificationPolicyKind.DIRECT_MESSAGE:
        return m('settings.notifications.policy.reason.direct_message');
      case NotificationPolicyKind.DIRECT_MENTION:
        return m('settings.notifications.policy.reason.direct_mention');
      case NotificationPolicyKind.REPLY:
        return m('settings.notifications.policy.reason.reply');
      case NotificationPolicyKind.ROLE_MENTION:
        return m('settings.notifications.policy.reason.role_mention');
      case NotificationPolicyKind.HERE:
        return m('settings.notifications.policy.reason.here');
      case NotificationPolicyKind.ALL:
        return m('settings.notifications.policy.reason.all');
      case NotificationPolicyKind.FOLLOWED_THREAD:
        return m('settings.notifications.policy.reason.followed_thread');
      case NotificationPolicyKind.FOLLOWED_ROOM:
        return m('settings.notifications.policy.reason.followed_room');
      case NotificationPolicyKind.REACTION:
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
  <select
    class="input mb-3 w-full text-sm"
    aria-label={m('settings.notifications.policy.title')}
    value={selectedRoomId}
    disabled={savingReason !== null}
    onchange={(event) => {
      selectedRoomId = event.currentTarget.value;
    }}
  >
    <option value="">{serverScope.store.serverInfo.name}</option>
    {#each policyRooms as room (room.id)}
      <option value={room.id}>#{room.name}</option>
    {/each}
  </select>
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
            value={String(
              selectedRoomId
                ? (preference?.roomIntensity ?? NotificationDeliveryIntensity.UNSPECIFIED)
                : (preference?.serverIntensity ?? NotificationDeliveryIntensity.UNSPECIFIED)
            )}
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
