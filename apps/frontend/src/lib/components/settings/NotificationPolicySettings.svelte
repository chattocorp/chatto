<script lang="ts">
  import { FormSection, Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    NotificationDeliveryMode,
    NotificationPreferenceCategory,
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
  let savingKind = $state<NotificationPreferenceCategory | null>(null);
  let selectedRoomId = $state('');
  let loadGeneration = 0;

  const kinds = [
    NotificationPreferenceCategory.DIRECT_MESSAGE,
    NotificationPreferenceCategory.DIRECT_MENTION,
    NotificationPreferenceCategory.REPLY,
    NotificationPreferenceCategory.ROLE_MENTION,
    NotificationPreferenceCategory.HERE,
    NotificationPreferenceCategory.ALL,
    NotificationPreferenceCategory.FOLLOWED_THREAD,
    NotificationPreferenceCategory.FOLLOWED_ROOM,
    NotificationPreferenceCategory.REACTION
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

  async function change(kind: NotificationPreferenceCategory, event: Event) {
    const select = event.currentTarget as HTMLSelectElement;
    const preference = preferences.find((candidate) => candidate.category === kind);
    const roomId = selectedRoomId || undefined;
    const previousMode = preference?.override ?? NotificationDeliveryMode.UNSPECIFIED;
    const selectedMode = Number(select.value) as NotificationDeliveryMode;
    const override =
      selectedMode === NotificationDeliveryMode.UNSPECIFIED ? null : selectedMode;
    savingKind = kind;
    error = null;
    try {
      preferences = await notificationStore.setPolicyPreference(kind, override, roomId);
    } catch (cause) {
      select.value = String(previousMode);
      preferences = [...preferences];
      error =
        cause instanceof Error ? cause.message : m('settings.notifications.policy.save_failed');
    } finally {
      savingKind = null;
    }
  }

  function kindLabel(kind: NotificationPreferenceCategory): string {
    switch (kind) {
      case NotificationPreferenceCategory.DIRECT_MESSAGE:
        return m('settings.notifications.policy.reason.direct_message');
      case NotificationPreferenceCategory.DIRECT_MENTION:
        return m('settings.notifications.policy.reason.direct_mention');
      case NotificationPreferenceCategory.REPLY:
        return m('settings.notifications.policy.reason.reply');
      case NotificationPreferenceCategory.ROLE_MENTION:
        return m('settings.notifications.policy.reason.role_mention');
      case NotificationPreferenceCategory.HERE:
        return m('settings.notifications.policy.reason.here');
      case NotificationPreferenceCategory.ALL:
        return m('settings.notifications.policy.reason.all');
      case NotificationPreferenceCategory.FOLLOWED_THREAD:
        return m('settings.notifications.policy.reason.followed_thread');
      case NotificationPreferenceCategory.FOLLOWED_ROOM:
        return m('settings.notifications.policy.reason.followed_room');
      case NotificationPreferenceCategory.REACTION:
        return m('settings.notifications.policy.reason.reaction');
      default:
        return m('settings.notifications.policy.reason.activity');
    }
  }

  function intensityLabel(intensity: NotificationDeliveryMode): string {
    switch (intensity) {
      case NotificationDeliveryMode.OFF:
        return m('settings.notifications.policy.intensity.off');
      case NotificationDeliveryMode.BADGE:
        return m('settings.notifications.policy.intensity.badge');
      case NotificationDeliveryMode.ALERT:
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
    disabled={savingKind !== null}
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
      {#each kinds as kind (kind)}
        {@const preference = preferences.find((candidate) => candidate.category === kind)}
        <label class="flex items-center justify-between gap-4 px-3 py-3">
          <span class="min-w-0">
            <span class="block font-medium">{kindLabel(kind)}</span>
            <span class="block text-xs text-muted">
              {m('settings.notifications.policy.effective', {
                intensity: intensityLabel(
                  preference?.effective ?? NotificationDeliveryMode.OFF
                )
              })}
            </span>
          </span>
          <select
            class="input w-auto min-w-[120px] text-sm"
            aria-label={kindLabel(kind)}
            value={String(
              preference?.override ?? NotificationDeliveryMode.UNSPECIFIED
            )}
            disabled={savingKind !== null}
            onchange={(event) => change(kind, event)}
          >
            <option value={String(NotificationDeliveryMode.UNSPECIFIED)}
              >{intensityLabel(NotificationDeliveryMode.UNSPECIFIED)}</option
            >
            <option value={String(NotificationDeliveryMode.OFF)}
              >{intensityLabel(NotificationDeliveryMode.OFF)}</option
            >
            <option value={String(NotificationDeliveryMode.BADGE)}
              >{intensityLabel(NotificationDeliveryMode.BADGE)}</option
            >
            <option value={String(NotificationDeliveryMode.ALERT)}
              >{intensityLabel(NotificationDeliveryMode.ALERT)}</option
            >
          </select>
        </label>
      {/each}
    </div>
  {/if}
</FormSection>
