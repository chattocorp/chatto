<!--
@component

Scope-wide notification presets for room and server context menus. Each action
updates every built-in notification class atomically; mixed per-class policies
remain available on the full notification settings page.
-->
<script lang="ts">
  import {
    NotificationDeliveryMode,
    type NotificationPolicy,
    type NotificationPolicyModes
  } from '$lib/api-client/notifications';
  import { m } from '$lib/i18n/messages';
  import type { NotificationStore } from '$lib/state/server/notifications.svelte';
  import { toast } from '$lib/ui/toast';

  let {
    notificationStore,
    roomId,
    onupdated = () => {}
  }: {
    notificationStore: Pick<NotificationStore, 'getPolicy' | 'updatePolicy'>;
    roomId?: string;
    onupdated?: () => void;
  } = $props();

  let saving = $state(false);
  const policyRequest = $derived(notificationStore.getPolicy(roomId));

  function selectedPreset(policy: NotificationPolicy | null): NotificationDeliveryMode | null {
    if (!policy) return null;
    const modes = Object.values(policy.effective);
    if (modes.every((mode) => mode === NotificationDeliveryMode.ALERT)) {
      return NotificationDeliveryMode.ALERT;
    }
    if (modes.every((mode) => mode === NotificationDeliveryMode.SILENT)) {
      return NotificationDeliveryMode.SILENT;
    }
    if (modes.every((mode) => mode === NotificationDeliveryMode.OFF)) {
      return NotificationDeliveryMode.OFF;
    }
    return null;
  }

  function presetPatch(mode: NotificationDeliveryMode): NotificationPolicyModes {
    return {
      directMessages: mode,
      directMentions: mode,
      replies: mode,
      roleMentions: mode,
      hereMentions: mode,
      allMentions: mode,
      followedThreads: mode,
      followedRooms: mode,
      reactions: mode
    };
  }

  async function applyPreset(mode: NotificationDeliveryMode): Promise<void> {
    if (saving) return;
    saving = true;
    try {
      await notificationStore.updatePolicy(presetPatch(mode), roomId);
      toast.success(m('common.saved'));
      onupdated();
    } catch {
      toast.error(m('settings.notifications.policy.save_failed'));
    } finally {
      saving = false;
    }
  }
</script>

{#snippet preset(
  mode: NotificationDeliveryMode,
  label: string,
  icon: string,
  currentPreset: NotificationDeliveryMode | null
)}
  <button
    type="button"
    class="sidebar-item disabled:cursor-not-allowed disabled:opacity-50"
    onclick={() => void applyPreset(mode)}
    disabled={saving}
    role="menuitemradio"
    aria-checked={currentPreset === mode}
  >
    <span
      class={[
        'iconify sidebar-icon',
        currentPreset === mode ? 'icon-[uil--check]' : icon
      ]}
      aria-hidden="true"
    ></span>
    {label}
  </button>
{/snippet}

{#snippet actions(currentPolicy: NotificationPolicy | null)}
  {@const currentPreset = selectedPreset(currentPolicy)}
  <div role="separator" class="mx-2 my-1 border-t border-text/10"></div>
  <div class="px-3 py-1.5 font-medium text-muted" role="presentation">
    {m('settings.nav.notifications')}
  </div>
  {@render preset(
    NotificationDeliveryMode.ALERT,
    m('settings.notifications.policy.delivery_mode.alert'),
    'icon-[uil--bell]',
    currentPreset
  )}
  {@render preset(
    NotificationDeliveryMode.SILENT,
    m('settings.notifications.policy.delivery_mode.silent'),
    'icon-[uil--volume-mute]',
    currentPreset
  )}
  {@render preset(
    NotificationDeliveryMode.OFF,
    m('settings.notifications.policy.delivery_mode.off'),
    'icon-[uil--bell-slash]',
    currentPreset
  )}
{/snippet}

{#await policyRequest}
  {@render actions(null)}
{:then currentPolicy}
  {@render actions(currentPolicy)}
{:catch}
  <!-- Current-state lookup is optional; preset mutations remain available. -->
  {@render actions(null)}
{/await}
