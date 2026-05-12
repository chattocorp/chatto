<script lang="ts">
  import { PaneHeader, Hint } from '$lib/ui';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import {
    notificationSounds,
    playNotificationSound,
    soundCategories,
    type NotificationSoundId,
    type SoundCategory
  } from '$lib/audio/notificationSounds';
  import {
    isSupported as isPushSupported,
    isSubscribed as checkPushSubscription,
    subscribe as subscribeToPush,
    unsubscribe as unsubscribeFromPush
  } from '$lib/notifications/pushNotifications';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const getServerId = getActiveServer();
  const serverInfo = serverRegistry.getStore(getServerId()).serverInfo;

  function selectSound(soundId: NotificationSoundId) {
    userPreferences.notificationSound = soundId;
    if (soundId !== 'silent') {
      playNotificationSound(soundId);
    }
  }

  function getSoundsForCategory(category: SoundCategory) {
    return notificationSounds.filter((s) => s.category === category);
  }

  // Push notifications state
  let pushEnabled = $derived(serverInfo.pushNotificationsEnabled);
  let pushSupported = isPushSupported();
  let pushSubscribed = $state(false);
  let pushLoading = $state(false);
  let pushError = $state<string | null>(null);

  // Check push subscription status on mount
  $effect(() => {
    if (pushEnabled && pushSupported) {
      checkPushSubscription().then((subscribed) => {
        pushSubscribed = subscribed;
      });
    }
  });

  async function handleEnablePush() {
    const vapidKey = serverInfo.vapidPublicKey;
    if (!vapidKey) {
      pushError = 'Push notifications are not configured on this server';
      return;
    }

    pushLoading = true;
    pushError = null;

    try {
      const success = await subscribeToPush(vapidKey);
      if (success) {
        pushSubscribed = true;
      } else {
        pushError = 'Failed to enable push notifications. Please try again.';
      }
    } catch {
      pushError = 'An error occurred while enabling push notifications';
    } finally {
      pushLoading = false;
    }
  }

  async function handleDisablePush() {
    pushLoading = true;
    pushError = null;

    try {
      const success = await unsubscribeFromPush();
      if (success) {
        pushSubscribed = false;
      } else {
        pushError = 'Failed to disable push notifications';
      }
    } catch {
      pushError = 'An error occurred while disabling push notifications';
    } finally {
      pushLoading = false;
    }
  }
</script>

<PaneHeader
  title="Notifications"
  subtitle="Configure how you receive notifications"
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Push Notifications Section (only show if enabled on server) -->
  {#if pushEnabled}
    <div class="max-w-lg">
      <h3 class="mb-4 text-sm font-semibold text-muted">Push Notifications</h3>

      {#if !pushSupported}
        <div class="rounded-lg border border-border bg-surface-100 px-4 py-3 text-sm text-muted">
          Push notifications are not supported in this browser.
        </div>
      {:else if pushError}
        <div class="mb-3">
          <Hint tone="danger">{pushError}</Hint>
        </div>
      {/if}

      {#if pushSupported}
        {#if pushSubscribed}
          <div
            class="flex items-center justify-between rounded-lg border border-accent bg-accent/10 px-4 py-3"
          >
            <div>
              <p class="font-medium text-accent">Push notifications enabled</p>
              <p class="mt-1 text-sm text-muted">
                You'll receive notifications even when the browser is closed.
              </p>
            </div>
            <button
              type="button"
              class="cursor-pointer rounded-lg border border-border bg-surface-100 px-3 py-1.5 text-sm transition-colors hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
              onclick={handleDisablePush}
              disabled={pushLoading}
            >
              {pushLoading ? 'Disabling...' : 'Disable'}
            </button>
          </div>
        {:else}
          <div
            class="flex items-center justify-between rounded-lg border border-border bg-surface-100 px-4 py-3"
          >
            <div>
              <p class="font-medium">Enable push notifications</p>
              <p class="mt-1 text-sm text-muted">
                Get notified about new messages even when the browser is closed.
              </p>
            </div>
            <button
              type="button"
              class="cursor-pointer rounded-lg bg-accent px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
              onclick={handleEnablePush}
              disabled={pushLoading}
            >
              {pushLoading ? 'Enabling...' : 'Enable'}
            </button>
          </div>
        {/if}
      {/if}
    </div>
  {/if}

  <!-- Notification Sound Section -->
  <div class="max-w-lg">
    <h3 class="mb-4 text-sm font-semibold text-muted">Notification Sound</h3>

    <div class="flex flex-col gap-4">
      {#each soundCategories as category (category)}
        {@const sounds = getSoundsForCategory(category)}
        <div>
          <h4 class="mb-2 text-xs font-medium tracking-wide text-muted/70 uppercase">
            {category}
          </h4>
          <div class="flex flex-col gap-1">
            {#each sounds as sound (sound.id)}
              {@const isSelected = userPreferences.notificationSound === sound.id}
              <button
                type="button"
                class={[
                  'flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2 transition-colors',
                  isSelected
                    ? 'border-accent bg-accent/10'
                    : 'hover:border-border-highlighted border-border hover:bg-surface-100'
                ]}
                onclick={() => selectSound(sound.id)}
              >
                <span
                  class={[
                    'flex h-5 w-5 items-center justify-center rounded-full border-2 transition-colors',
                    isSelected ? 'border-accent bg-accent' : 'border-muted'
                  ]}
                >
                  {#if isSelected}
                    <span class="h-2 w-2 rounded-full bg-white"></span>
                  {/if}
                </span>
                <span class={isSelected ? 'font-medium' : ''}>{sound.name}</span>
              </button>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>
