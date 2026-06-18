<script lang="ts">
  import { PaneHeader, Hint, FormSection } from '$lib/ui';
  import NotificationLevelSettings from '$lib/components/settings/NotificationLevelSettings.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import {
    notificationSounds,
    playNotificationSound,
    soundCategories,
    type NotificationSoundFilters,
    type NotificationSoundId,
    type SoundCategory
  } from '$lib/audio/notificationSounds';
  import {
    ensureRegistered,
    getPermission,
    isSupported as isPushSupported,
    isSubscribed as checkPushSubscription
  } from '$lib/notifications/pushNotifications';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const serverInfo = serverRegistry.getStore(getActiveServer()).serverInfo;

  function selectSound(soundId: NotificationSoundId) {
    userPreferences.notificationSound = soundId;
    if (soundId !== 'silent') {
      playNotificationSound(soundId, userPreferences.notificationSoundFilters);
    }
  }

  function previewSelectedSound() {
    if (userPreferences.notificationSound === 'silent') return;
    playNotificationSound(
      userPreferences.notificationSound,
      userPreferences.notificationSoundFilters
    );
  }

  function updateSoundFilter(key: keyof NotificationSoundFilters, event: Event) {
    const value = Number((event.currentTarget as HTMLInputElement).value);
    userPreferences.setNotificationSoundFilter(key, value);
  }

  function updateMuffledFilter(event: Event) {
    const amount = Number((event.currentTarget as HTMLInputElement).value);
    userPreferences.setNotificationSoundFilter('lowPassHz', lowPassHzFromMuffledAmount(amount));
  }

  function lowPassHzFromMuffledAmount(amount: number) {
    return 20000 - (amount / 100) * (20000 - 800);
  }

  function muffledAmountFromLowPassHz(value: number) {
    return Math.round(((20000 - value) / (20000 - 800)) * 100);
  }

  function formatVolume(value: number) {
    return `${Math.round(value * 100)}%`;
  }

  function formatEffect(value: number) {
    if (value <= 0) return 'Off';
    return `${Math.round(value)}%`;
  }

  function formatTinny(value: number) {
    if (value <= 20) return 'Off';
    return `${Math.round(((value - 20) / (2000 - 20)) * 100)}%`;
  }

  function formatMuffled(value: number) {
    const amount = muffledAmountFromLowPassHz(value);
    if (amount <= 0) return 'Off';
    return `${amount}%`;
  }

  function getSoundsForCategory(category: SoundCategory) {
    return notificationSounds.filter((s) => s.category === category);
  }

  // Push notifications state
  let pushEnabled = $derived(serverInfo.pushNotificationsEnabled);
  let pushSupported = isPushSupported();
  let pushPermission = $state<NotificationPermission | null>(getPermission());
  let pushSubscribed = $state(false);
  let pushLoading = $state(false);
  let pushError = $state<string | null>(null);

  // Check push subscription status on mount
  $effect(() => {
    if (pushEnabled && pushSupported) {
      pushPermission = getPermission();
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
      const success = await ensureRegistered(vapidKey, { prompt: true });
      pushPermission = getPermission();
      if (success) {
        pushSubscribed = true;
      } else {
        pushError =
          pushPermission === 'denied'
            ? 'Push notifications are blocked in your browser or OS settings.'
            : 'Failed to enable push notifications. Please try again.';
      }
    } catch {
      pushError = 'An error occurred while enabling push notifications';
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
  <NotificationLevelSettings />

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
        {#if pushPermission === 'denied'}
          <div class="rounded-lg border border-warning/60 bg-warning/10 px-4 py-3">
            <p class="font-medium text-warning">Push notifications blocked</p>
            <p class="mt-1 text-sm text-muted">
              Enable notifications in your browser or OS settings, then open Chatto again.
            </p>
          </div>
        {:else if pushSubscribed}
          <div
            class="rounded-lg border border-accent bg-accent/10 px-4 py-3"
          >
            <div>
              <p class="font-medium text-accent">Push notifications enabled</p>
              <p class="mt-1 text-sm text-muted">
                Chatto uses your browser or OS notification permission as the switch. To turn
                notifications off, disable them for this site in your browser or OS settings.
              </p>
            </div>
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

  <FormSection title="Sound Shape" maxWidth="max-w-lg" bordered>
    {#snippet actions()}
      <button
        type="button"
        class="cursor-pointer rounded-lg border border-border bg-surface-100 px-3 py-1.5 text-sm transition-colors hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
        onclick={previewSelectedSound}
        disabled={userPreferences.notificationSound === 'silent'}
      >
        Preview
      </button>
      <button
        type="button"
        class="hover:text-foreground cursor-pointer rounded-lg border border-border px-3 py-1.5 text-sm text-muted transition-colors hover:bg-surface-100"
        onclick={() => userPreferences.resetNotificationSoundFilters()}
      >
        Reset
      </button>
    {/snippet}

    <div class="flex flex-col gap-2">
      <label class="flex flex-col gap-2 rounded-lg border border-border px-3 py-2">
        <span class="flex items-center justify-between gap-3 text-sm">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span class="iconify shrink-0 text-base text-muted uil--volume" aria-hidden="true"
            ></span>
            <span>Volume</span>
          </span>
          <span class="text-muted tabular-nums">
            {formatVolume(userPreferences.notificationSoundFilters.volume)}
          </span>
        </span>
        <input
          data-testid="notification-volume-filter"
          type="range"
          min="0"
          max="2"
          step="0.05"
          value={userPreferences.notificationSoundFilters.volume}
          oninput={(event) => updateSoundFilter('volume', event)}
          onchange={previewSelectedSound}
          class="w-full cursor-pointer accent-accent"
        />
      </label>

      <label class="flex flex-col gap-2 rounded-lg border border-border px-3 py-2">
        <span class="flex items-center justify-between gap-3 text-sm">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span class="iconify shrink-0 text-base text-muted uil--bolt" aria-hidden="true"></span>
            <span>Tinny</span>
          </span>
          <span class="text-muted tabular-nums">
            {formatTinny(userPreferences.notificationSoundFilters.highPassHz)}
          </span>
        </span>
        <input
          data-testid="notification-high-pass-filter"
          type="range"
          min="20"
          max="2000"
          step="10"
          value={userPreferences.notificationSoundFilters.highPassHz}
          oninput={(event) => updateSoundFilter('highPassHz', event)}
          onchange={previewSelectedSound}
          class="w-full cursor-pointer accent-accent"
        />
      </label>

      <label class="flex flex-col gap-2 rounded-lg border border-border px-3 py-2">
        <span class="flex items-center justify-between gap-3 text-sm">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span class="iconify shrink-0 text-base text-muted uil--volume-mute" aria-hidden="true"
            ></span>
            <span>Muffled</span>
          </span>
          <span class="text-muted tabular-nums">
            {formatMuffled(userPreferences.notificationSoundFilters.lowPassHz)}
          </span>
        </span>
        <input
          data-testid="notification-low-pass-filter"
          type="range"
          min="0"
          max="100"
          step="1"
          value={muffledAmountFromLowPassHz(userPreferences.notificationSoundFilters.lowPassHz)}
          oninput={updateMuffledFilter}
          onchange={previewSelectedSound}
          class="w-full cursor-pointer accent-accent"
        />
      </label>

      <label class="flex flex-col gap-2 rounded-lg border border-border px-3 py-2">
        <span class="flex items-center justify-between gap-3 text-sm">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span class="iconify shrink-0 text-base text-muted uil--redo" aria-hidden="true"></span>
            <span>Echo</span>
          </span>
          <span class="text-muted tabular-nums">
            {formatEffect(userPreferences.notificationSoundFilters.echo)}
          </span>
        </span>
        <input
          data-testid="notification-echo-filter"
          type="range"
          min="0"
          max="100"
          step="1"
          value={userPreferences.notificationSoundFilters.echo}
          oninput={(event) => updateSoundFilter('echo', event)}
          onchange={previewSelectedSound}
          class="w-full cursor-pointer accent-accent"
        />
      </label>

      <label class="flex flex-col gap-2 rounded-lg border border-border px-3 py-2">
        <span class="flex items-center justify-between gap-3 text-sm">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span class="iconify shrink-0 text-base text-muted uil--cloud" aria-hidden="true"
            ></span>
            <span>Reverb</span>
          </span>
          <span class="text-muted tabular-nums">
            {formatEffect(userPreferences.notificationSoundFilters.reverb)}
          </span>
        </span>
        <input
          data-testid="notification-reverb-filter"
          type="range"
          min="0"
          max="100"
          step="1"
          value={userPreferences.notificationSoundFilters.reverb}
          oninput={(event) => updateSoundFilter('reverb', event)}
          onchange={previewSelectedSound}
          class="w-full cursor-pointer accent-accent"
        />
      </label>

      <label class="flex flex-col gap-2 rounded-lg border border-border px-3 py-2">
        <span class="flex items-center justify-between gap-3 text-sm">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span class="iconify shrink-0 text-base text-muted uil--fire" aria-hidden="true"></span>
            <span>Crunch</span>
          </span>
          <span class="text-muted tabular-nums">
            {formatEffect(userPreferences.notificationSoundFilters.crunch)}
          </span>
        </span>
        <input
          data-testid="notification-crunch-filter"
          type="range"
          min="0"
          max="100"
          step="1"
          value={userPreferences.notificationSoundFilters.crunch}
          oninput={(event) => updateSoundFilter('crunch', event)}
          onchange={previewSelectedSound}
          class="w-full cursor-pointer accent-accent"
        />
      </label>
    </div>
  </FormSection>
</div>
