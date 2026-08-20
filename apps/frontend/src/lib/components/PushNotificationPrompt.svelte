<!--
@component

Shows a persistent top-overlay prompt for users who can enable Web Push but
have not made a browser permission choice yet.
-->
<script lang="ts">
  import {
    ensureRegistered,
    getPushCapability,
    getPermission,
    refreshPushSubscriptions
  } from '$lib/notifications/pushNotifications';
  import { Codecs, serverSlot } from '$lib/storage/slot';
  import { TopOverlayNotice } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';

  let {
    serverId,
    userId,
    vapidPublicKey
  }: { serverId: string; userId: string; vapidPublicKey: string } = $props();

  // svelte-ignore state_referenced_locally
  const dismissedSlot = serverSlot(
    serverId,
    `user:${userId}:pushPromptDismissed`,
    false,
    Codecs.boolean
  );

  let dismissed = $state(dismissedSlot.get());
  let permission = $state<NotificationPermission | null>(getPermission());
  let loading = $state(false);

  const pushCapability = getPushCapability();
  const supported = pushCapability === 'supported';
  const needsIosHomeScreen = pushCapability === 'ios_home_screen_required';
  const canShowPushPrompt = $derived(Boolean(vapidPublicKey && !dismissed));
  const shouldShowEnablePrompt = $derived(
    canShowPushPrompt && supported && permission === 'default'
  );
  const shouldShowIosHomeScreenNotice = $derived(canShowPushPrompt && needsIosHomeScreen);

  function optOut() {
    dismissed = true;
    dismissedSlot.set(true);
  }

  async function enablePush() {
    if (!vapidPublicKey) return;

    loading = true;
    try {
      const enabled = await ensureRegistered(serverId, vapidPublicKey, { prompt: true });
      permission = getPermission();

      if (enabled) {
        await refreshPushSubscriptions();
        toast.success(m('settings.notifications.push_prompt.enabled'));
        return;
      }

      if (permission === 'denied') {
        toast.warning(m('settings.notifications.push_prompt.blocked'));
      } else {
        toast.error(m('settings.notifications.push_prompt.enable_failed'));
      }
    } finally {
      loading = false;
    }
  }
</script>

{#if shouldShowEnablePrompt}
  <TopOverlayNotice
    title={m('settings.notifications.push_prompt.title')}
    message={m('settings.notifications.push_prompt.message')}
    icon="icon-[uil--bell]"
    tone="info"
    {loading}
    primaryAction={{
      label: loading
        ? m('settings.notifications.push_prompt.enabling')
        : m('settings.notifications.push_prompt.enable'),
      icon: 'icon-[uil--bell]',
      onclick: enablePush
    }}
    secondaryAction={{
      label: m('settings.notifications.push_prompt.dismiss'),
      onclick: optOut
    }}
  />
{:else if shouldShowIosHomeScreenNotice}
  <TopOverlayNotice
    title={m('settings.notifications.push_prompt.ios_home_screen_title')}
    message={m('settings.notifications.push_prompt.ios_home_screen_message')}
    icon="icon-[uil--mobile-android]"
    tone="info"
    secondaryAction={{
      label: m('settings.notifications.push_prompt.dismiss'),
      onclick: optOut
    }}
  />
{/if}
