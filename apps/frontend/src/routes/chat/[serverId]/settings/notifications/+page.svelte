<script lang="ts">
  import { PaneHeader, Hint } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import NotificationLevelSettings from '$lib/components/settings/NotificationLevelSettings.svelte';
  import {
    ensureRegistered,
    getPushCapability,
    getPermission,
    isSubscribed as checkPushSubscription,
    sendTestNotification
  } from '$lib/notifications/pushNotifications';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import * as m from '$lib/i18n/messages';

  const activeServerId = $derived(getActiveServer());
  const serverInfo = $derived(serverRegistry.getStore(activeServerId).serverInfo);
  const isOriginServer = $derived(serverRegistry.isOriginServer(activeServerId));

  // Push notifications state
  let pushEnabled = $derived(serverInfo.pushNotificationsEnabled);
  let showOriginPushControls = $derived(pushEnabled && isOriginServer);
  let showRemotePushNotice = $derived(pushEnabled && !isOriginServer);
  const pushCapability = getPushCapability();
  const pushSupported = pushCapability === 'supported';
  const needsIosHomeScreen = pushCapability === 'ios_home_screen_required';
  let pushPermission = $state<NotificationPermission | null>(getPermission());
  let pushSubscribed = $state(false);
  let pushLoading = $state(false);
  let pushError = $state<string | null>(null);
  let pushTestLoading = $state(false);
  let pushTestStatus = $state<'sent' | 'failed' | null>(null);

  // Check push subscription status on mount
  $effect(() => {
    if (showOriginPushControls && pushSupported) {
      pushPermission = getPermission();
      checkPushSubscription().then((subscribed) => {
        pushSubscribed = subscribed;
      });
    }
  });

  async function handleEnablePush() {
    const vapidKey = serverInfo.vapidPublicKey;
    if (!vapidKey) {
      pushError = m['settings.notifications.push.not_configured']();
      return;
    }

    pushLoading = true;
    pushError = null;

    try {
      const success = await ensureRegistered(vapidKey, { prompt: true });
      pushPermission = getPermission();
      if (success) {
        pushSubscribed = true;
      } else {
        pushError =
          pushPermission === 'denied'
            ? m['settings.notifications.push.blocked_error']()
            : m['settings.notifications.push.enable_failed']();
      }
    } catch {
      pushError = m['settings.notifications.push.enable_error']();
    } finally {
      pushLoading = false;
    }
  }

  async function handleTestPush() {
    pushTestLoading = true;
    pushTestStatus = null;
    try {
      pushTestStatus = (await sendTestNotification()) ? 'sent' : 'failed';
    } catch {
      pushTestStatus = 'failed';
    } finally {
      pushTestLoading = false;
    }
  }
</script>

<PaneHeader
  title={m['settings.notifications.title']()}
  subtitle={m['settings.notifications.subtitle']()}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <NotificationLevelSettings />

  <!-- Push Notifications Section (only show if enabled on server) -->
  {#if showRemotePushNotice}
    <div class="max-w-lg">
      <h3 class="mb-4 text-sm font-semibold text-muted">
        {m['settings.notifications.push.title']()}
      </h3>
      <Hint tone="info">
        <div>
          <p class="font-medium">{m['settings.notifications.push.remote_title']()}</p>
          <p class="mt-1 text-sm text-muted">
            {m['settings.notifications.push.remote_description']()}
          </p>
        </div>
      </Hint>
    </div>
  {:else if showOriginPushControls}
    <div class="max-w-lg">
      <h3 class="mb-4 text-sm font-semibold text-muted">
        {m['settings.notifications.push.title']()}
      </h3>

      {#if needsIosHomeScreen}
        <Hint tone="info">
          <div>
            <p class="font-medium">{m['settings.notifications.push.ios_home_screen_title']()}</p>
            <p class="mt-1 text-sm text-muted">
              {m['settings.notifications.push.ios_home_screen_description']()}
            </p>
          </div>
        </Hint>
      {:else if !pushSupported}
        <div class="surface-box px-4 py-3 text-sm text-muted">
          {m['settings.notifications.push.not_supported']()}
        </div>
      {:else if pushError}
        <div class="mb-3">
          <Hint tone="danger">{pushError}</Hint>
        </div>
      {/if}

      {#if pushSupported}
        {#if pushPermission === 'denied'}
          <div class="rounded-lg border border-warning/60 bg-warning/10 px-4 py-3">
            <p class="font-medium text-warning">
              {m['settings.notifications.push.blocked_title']()}
            </p>
            <p class="mt-1 text-sm text-muted">
              {m['settings.notifications.push.blocked_description']()}
            </p>
          </div>
        {:else if pushSubscribed}
          <div class="flex flex-col gap-3">
            <Hint tone="success">
              <div>
                <p class="font-medium">{m['settings.notifications.push.enabled_title']()}</p>
                <p class="mt-1 text-sm text-muted">
                  {m['settings.notifications.push.enabled_description']()}
                </p>
              </div>
            </Hint>
            <div class="flex items-center gap-3">
              <Button
                variant="secondary"
                size="sm"
                onclick={handleTestPush}
                disabled={pushTestLoading}
                loading={pushTestLoading}
                loadingText={m['settings.notifications.push.testing']()}
              >
                {m['settings.notifications.push.test_button']()}
              </Button>
              {#if pushTestStatus === 'sent'}
                <span class="text-sm text-success" role="status">
                  {m['settings.notifications.push.test_sent']()}
                </span>
              {:else if pushTestStatus === 'failed'}
                <span class="text-sm text-danger" role="alert">
                  {m['settings.notifications.push.test_failed']()}
                </span>
              {/if}
            </div>
          </div>
        {:else}
          <div class="flex items-center justify-between surface-box px-4 py-3">
            <div>
              <p class="font-medium">{m['settings.notifications.push.enable_title']()}</p>
              <p class="mt-1 text-sm text-muted">
                {m['settings.notifications.push.enable_description']()}
              </p>
            </div>
            <Button
              variant="action"
              size="sm"
              onclick={handleEnablePush}
              disabled={pushLoading}
              loading={pushLoading}
              loadingText={m['settings.notifications.push.enabling']()}
            >
              {m['settings.notifications.push.enable_button']()}
            </Button>
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
