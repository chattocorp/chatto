<script lang="ts">
  import { FormSection, Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    NotificationDeliveryMode,
    type NotificationPolicy,
    type NotificationPolicyField,
    type NotificationPolicyPatch
  } from '$lib/api-client/notifications';

  const serverScope = useServerScope();
  const notificationStore = $derived(serverScope.store.notifications);
  const policyRooms = $derived(
    (serverScope.store.navigation?.rooms ?? []).filter((room) => room.viewerIsMember)
  );
  let policy = $state.raw<NotificationPolicy | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let savingField = $state<NotificationPolicyField | null>(null);
  let selectedRoomId = $state('');
  let loadGeneration = 0;

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
      const nextPolicy = await notificationStore.getPolicy(roomId || undefined);
      if (generation !== loadGeneration) return;
      policy = nextPolicy;
    } catch (cause) {
      if (generation !== loadGeneration) return;
      error =
        cause instanceof Error ? cause.message : m('settings.notifications.policy.load_failed');
    } finally {
      if (generation === loadGeneration) loading = false;
    }
  }

  async function change(field: NotificationPolicyField, event: Event) {
    const select = event.currentTarget as HTMLSelectElement;
    const roomId = selectedRoomId || undefined;
    const previousMode = policy?.overrides[field] ?? NotificationDeliveryMode.UNSPECIFIED;
    const selectedMode = Number(select.value) as NotificationDeliveryMode;
    const override = selectedMode === NotificationDeliveryMode.UNSPECIFIED ? null : selectedMode;
    const patch: NotificationPolicyPatch = {};
    patch[field] = override;
    savingField = field;
    error = null;
    try {
      policy = await notificationStore.updatePolicy(patch, roomId);
    } catch (cause) {
      select.value = String(previousMode);
      if (policy) policy = { ...policy };
      error =
        cause instanceof Error ? cause.message : m('settings.notifications.policy.save_failed');
    } finally {
      savingField = null;
    }
  }

  function deliveryModeLabel(mode: NotificationDeliveryMode): string {
    switch (mode) {
      case NotificationDeliveryMode.OFF:
        return m('settings.notifications.policy.delivery_mode.off');
      case NotificationDeliveryMode.SILENT:
        return m('settings.notifications.policy.delivery_mode.silent');
      case NotificationDeliveryMode.ALERT:
        return m('settings.notifications.policy.delivery_mode.alert');
      default:
        return m('settings.notifications.policy.delivery_mode.inherit');
    }
  }
</script>

{#snippet policyRow(field: NotificationPolicyField, label: string)}
  <label class="flex items-center justify-between gap-4 px-3 py-3">
    <span class="min-w-0">
      <span class="block font-medium">{label}</span>
      <span class="block text-xs text-muted">
        {m('settings.notifications.policy.effective', {
          mode: deliveryModeLabel(policy?.effective[field] ?? NotificationDeliveryMode.OFF)
        })}
      </span>
    </span>
    <select
      class="input w-auto min-w-[120px] text-sm"
      aria-label={label}
      value={String(policy?.overrides[field] ?? NotificationDeliveryMode.UNSPECIFIED)}
      disabled={savingField !== null}
      onchange={(event) => change(field, event)}
    >
      <option value={String(NotificationDeliveryMode.UNSPECIFIED)}
        >{deliveryModeLabel(NotificationDeliveryMode.UNSPECIFIED)}</option
      >
      <option value={String(NotificationDeliveryMode.OFF)}
        >{deliveryModeLabel(NotificationDeliveryMode.OFF)}</option
      >
      <option value={String(NotificationDeliveryMode.SILENT)}
        >{deliveryModeLabel(NotificationDeliveryMode.SILENT)}</option
      >
      <option value={String(NotificationDeliveryMode.ALERT)}
        >{deliveryModeLabel(NotificationDeliveryMode.ALERT)}</option
      >
    </select>
  </label>
{/snippet}

<FormSection title={m('settings.notifications.policy.title')} maxWidth="max-w-2xl" bordered>
  <p class="mb-3 text-sm text-muted">{m('settings.notifications.policy.description')}</p>
  <select
    class="mb-3 input w-full text-sm"
    aria-label={m('settings.notifications.policy.title')}
    value={selectedRoomId}
    disabled={savingField !== null}
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
  {:else if policy}
    <div class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {@render policyRow(
        'directMessages',
        m('settings.notifications.policy.reason.direct_message')
      )}
      {@render policyRow(
        'directMentions',
        m('settings.notifications.policy.reason.direct_mention')
      )}
      {@render policyRow('replies', m('settings.notifications.policy.reason.reply'))}
      {@render policyRow('roleMentions', m('settings.notifications.policy.reason.role_mention'))}
      {@render policyRow('hereMentions', m('settings.notifications.policy.reason.here'))}
      {@render policyRow('allMentions', m('settings.notifications.policy.reason.all'))}
      {@render policyRow(
        'followedThreads',
        m('settings.notifications.policy.reason.followed_thread')
      )}
      {@render policyRow('followedRooms', m('settings.notifications.policy.reason.followed_room'))}
      {@render policyRow('reactions', m('settings.notifications.policy.reason.reaction'))}
    </div>
  {/if}
</FormSection>
