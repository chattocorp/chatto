<!--
@component

Shows a persistent top-overlay prompt for users who can enable Web Push but
have not made a browser permission choice yet.
-->
<script lang="ts">
  import {
    ensureRegistered,
    getPermission,
    isSupported as isPushSupported
  } from '$lib/notifications/pushNotifications';
  import { Codecs, serverSlot } from '$lib/storage/slot';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { TopOverlayNotice } from '$lib/ui';
  import { toast } from '$lib/ui/toast';

  let { userId }: { userId: string } = $props();

  const originId = serverRegistry.originServer?.id ?? '';
  const originServerInfo = originId ? serverRegistry.getStore(originId).serverInfo : undefined;
  // svelte-ignore state_referenced_locally
  const dismissedSlot = serverSlot(
    originId,
    `user:${userId}:pushPromptDismissed`,
    false,
    Codecs.boolean
  );

  let dismissed = $state(dismissedSlot.get());
  let permission = $state<NotificationPermission | null>(getPermission());
  let loading = $state(false);

  const supported = isPushSupported();
  const vapidKey = $derived(originServerInfo?.vapidPublicKey ?? null);
  const shouldShow = $derived(
    Boolean(
      originServerInfo?.pushNotificationsEnabled &&
        vapidKey &&
        supported &&
        permission === 'default' &&
        !dismissed
    )
  );

  function optOut() {
    dismissed = true;
    dismissedSlot.set(true);
  }

  async function enablePush() {
    if (!vapidKey) return;

    loading = true;
    try {
      const enabled = await ensureRegistered(vapidKey, { prompt: true });
      permission = getPermission();

      if (enabled) {
        toast.success('Push notifications enabled');
        return;
      }

      if (permission === 'denied') {
        toast.warning('Push notifications are blocked in your browser or OS settings');
      } else {
        toast.error('Failed to enable push notifications');
      }
    } finally {
      loading = false;
    }
  }
</script>

{#if shouldShow}
  <TopOverlayNotice
    title="Enable push notifications"
    message="Get notified about DMs, mentions, and replies."
    icon="uil--bell"
    tone="info"
    loading={loading}
    primaryAction={{
      label: loading ? 'Enabling...' : 'Enable',
      icon: 'uil--bell',
      onclick: enablePush
    }}
    secondaryAction={{
      label: 'No thanks',
      onclick: optOut
    }}
  />
{/if}
