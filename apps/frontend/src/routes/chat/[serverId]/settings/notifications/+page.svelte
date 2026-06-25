<script lang="ts">
  import { PaneHeader, Hint, FormSection } from '$lib/ui';
  import { Button } from '$lib/ui/form';
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
  import * as m from '$lib/i18n/messages';

  const activeServerId = $derived(getActiveServer());
  const serverInfo = $derived(serverRegistry.getStore(activeServerId).serverInfo);
  const isOriginServer = $derived(serverRegistry.isOriginServer(activeServerId));

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
    if (value <= 0) return m['settings.notifications.sound.off']();
    return `${Math.round(value)}%`;
  }

  function formatTinny(value: number) {
    if (value <= 20) return m['settings.notifications.sound.off']();
    return `${Math.round(((value - 20) / (2000 - 20)) * 100)}%`;
  }

  function formatMuffled(value: number) {
    const amount = muffledAmountFromLowPassHz(value);
    if (amount <= 0) return m['settings.notifications.sound.off']();
    return `${amount}%`;
  }

  function getSoundsForCategory(category: SoundCategory) {
    return notificationSounds.filter((s) => s.category === category);
  }

  function soundCategoryLabel(category: SoundCategory) {
    switch (category) {
      case 'Silent':
        return m['settings.notifications.sound.category.silent']();
      case 'Simple':
        return m['settings.notifications.sound.category.simple']();
      case 'Playful':
        return m['settings.notifications.sound.category.playful']();
      case 'Robots':
        return m['settings.notifications.sound.category.robots']();
      case 'Musical':
        return m['settings.notifications.sound.category.musical']();
      case 'Here Be Dragons':
        return m['settings.notifications.sound.category.here_be_dragons']();
    }
  }

  function soundNameLabel(soundId: NotificationSoundId) {
    switch (soundId) {
      case 'silent':
        return m['settings.notifications.sound.name.silent']();
      case 'ding':
        return m['settings.notifications.sound.name.ding']();
      case 'chime-up':
        return m['settings.notifications.sound.name.chime_up']();
      case 'chime-down':
        return m['settings.notifications.sound.name.chime_down']();
      case 'pop':
        return m['settings.notifications.sound.name.pop']();
      case 'bubble':
        return m['settings.notifications.sound.name.bubble']();
      case 'retro':
        return m['settings.notifications.sound.name.retro']();
      case 'coin':
        return m['settings.notifications.sound.name.coin']();
      case 'powerup':
        return m['settings.notifications.sound.name.powerup']();
      case 'fanfare':
        return m['settings.notifications.sound.name.fanfare']();
      case 'laser':
        return m['settings.notifications.sound.name.laser']();
      case 'robot':
        return m['settings.notifications.sound.name.robot']();
      case 'ufo':
        return m['settings.notifications.sound.name.ufo']();
      case 'beepboop':
        return m['settings.notifications.sound.name.beepboop']();
      case 'dialup':
        return m['settings.notifications.sound.name.dialup']();
      case 'r2d2':
        return m['settings.notifications.sound.name.r2d2']();
      case 'harp':
        return m['settings.notifications.sound.name.harp']();
      case 'music-box':
        return m['settings.notifications.sound.name.music_box']();
      case 'celesta':
        return m['settings.notifications.sound.name.celesta']();
      case 'synth':
        return m['settings.notifications.sound.name.synth']();
      case 'orchestra':
        return m['settings.notifications.sound.name.orchestra']();
      case 'la-cucaracha':
        return m['settings.notifications.sound.name.la_cucaracha']();
      case 'chaos':
        return m['settings.notifications.sound.name.chaos']();
      case 'glitch':
        return m['settings.notifications.sound.name.glitch']();
      case 'siren':
        return m['settings.notifications.sound.name.siren']();
      case 'dubstep':
        return m['settings.notifications.sound.name.dubstep']();
      case 'circus':
        return m['settings.notifications.sound.name.circus']();
    }
  }

  // Push notifications state
  let pushEnabled = $derived(serverInfo.pushNotificationsEnabled);
  let showOriginPushControls = $derived(pushEnabled && isOriginServer);
  let showRemotePushNotice = $derived(pushEnabled && !isOriginServer);
  let pushSupported = isPushSupported();
  let pushPermission = $state<NotificationPermission | null>(getPermission());
  let pushSubscribed = $state(false);
  let pushLoading = $state(false);
  let pushError = $state<string | null>(null);

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

      {#if !pushSupported}
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
          <Hint tone="success">
            <div>
              <p class="font-medium">{m['settings.notifications.push.enabled_title']()}</p>
              <p class="mt-1 text-sm text-muted">
                {m['settings.notifications.push.enabled_description']()}
              </p>
            </div>
          </Hint>
        {:else}
          <div class="flex items-center justify-between surface-box px-4 py-3">
            <div>
              <p class="font-medium">{m['settings.notifications.push.enable_title']()}</p>
              <p class="mt-1 text-sm text-muted">
                {m['settings.notifications.push.enable_description']()}
              </p>
            </div>
            <Button
              variant="accent"
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

  <!-- Notification Sound Section -->
  <div class="max-w-lg">
    <h3 class="mb-4 text-sm font-semibold text-muted">
      {m['settings.notifications.sound.title']()}
    </h3>

    <div class="flex flex-col gap-4">
      {#each soundCategories as category (category)}
        {@const sounds = getSoundsForCategory(category)}
        <div>
          <h4 class="mb-2 text-xs font-medium tracking-wide text-muted/70 uppercase">
            {soundCategoryLabel(category)}
          </h4>
          <div class="flex flex-col gap-1">
            {#each sounds as sound (sound.id)}
              {@const isSelected = userPreferences.notificationSound === sound.id}
              <button
                type="button"
                class={['choice-row', isSelected && 'choice-row-selected']}
                onclick={() => selectSound(sound.id)}
              >
                <span class={['choice-indicator', isSelected && 'choice-indicator-selected']}>
                  {#if isSelected}
                    <span class="choice-indicator-dot"></span>
                  {/if}
                </span>
                <span class={isSelected ? 'font-medium' : ''}>{soundNameLabel(sound.id)}</span>
              </button>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>

  <FormSection title={m['settings.notifications.sound.shape_title']()} maxWidth="max-w-lg" bordered>
    {#snippet actions()}
      <Button
        variant="secondary"
        size="sm"
        onclick={previewSelectedSound}
        disabled={userPreferences.notificationSound === 'silent'}
      >
        {m['settings.notifications.sound.preview']()}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        onclick={() => userPreferences.resetNotificationSoundFilters()}
      >
        {m['settings.notifications.sound.reset']()}
      </Button>
    {/snippet}

    <div class="flex flex-col gap-2">
      <label class="flex flex-col gap-2 rounded-lg border border-border px-3 py-2">
        <span class="flex items-center justify-between gap-3 text-sm">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span class="iconify shrink-0 text-base text-muted uil--volume" aria-hidden="true"
            ></span>
            <span>{m['settings.notifications.sound.volume']()}</span>
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
            <span>{m['settings.notifications.sound.tinny']()}</span>
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
            <span>{m['settings.notifications.sound.muffled']()}</span>
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
            <span>{m['settings.notifications.sound.echo']()}</span>
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
            <span>{m['settings.notifications.sound.reverb']()}</span>
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
            <span>{m['settings.notifications.sound.crunch']()}</span>
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
