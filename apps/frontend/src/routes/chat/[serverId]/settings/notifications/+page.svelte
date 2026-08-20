<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { ChoiceRow, PaneHeader, Hint, FormSection } from '$lib/ui';
  import { Button, RangeField } from '$lib/ui/form';
  import NotificationPolicySettings from '$lib/components/settings/NotificationPolicySettings.svelte';
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
    isBrowserWebPushRuntime,
    getPushCapability,
    getPermission,
    isSubscribed as checkPushSubscription,
    refreshPushSubscriptions,
    sendTestNotification
  } from '$lib/notifications/pushNotifications';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();

  const activeServerId = $derived(serverScope.serverId);
  const serverInfo = $derived(serverScope.store.serverInfo);

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
    if (value <= 0) return m('settings.notifications.sound.off');
    return `${Math.round(value)}%`;
  }

  function formatTinny(value: number) {
    if (value <= 20) return m('settings.notifications.sound.off');
    return `${Math.round(((value - 20) / (2000 - 20)) * 100)}%`;
  }

  function formatMuffled(value: number) {
    const amount = muffledAmountFromLowPassHz(value);
    if (amount <= 0) return m('settings.notifications.sound.off');
    return `${amount}%`;
  }

  function getSoundsForCategory(category: SoundCategory) {
    return notificationSounds.filter((s) => s.category === category);
  }

  function soundCategoryLabel(category: SoundCategory) {
    switch (category) {
      case 'Silent':
        return m('settings.notifications.sound.category.silent');
      case 'Simple':
        return m('settings.notifications.sound.category.simple');
      case 'Playful':
        return m('settings.notifications.sound.category.playful');
      case 'Robots':
        return m('settings.notifications.sound.category.robots');
      case 'Musical':
        return m('settings.notifications.sound.category.musical');
      case 'Here Be Dragons':
        return m('settings.notifications.sound.category.here_be_dragons');
    }
  }

  function soundNameLabel(soundId: NotificationSoundId) {
    switch (soundId) {
      case 'silent':
        return m('settings.notifications.sound.name.silent');
      case 'ding':
        return m('settings.notifications.sound.name.ding');
      case 'chime-up':
        return m('settings.notifications.sound.name.chime_up');
      case 'chime-down':
        return m('settings.notifications.sound.name.chime_down');
      case 'pop':
        return m('settings.notifications.sound.name.pop');
      case 'bubble':
        return m('settings.notifications.sound.name.bubble');
      case 'retro':
        return m('settings.notifications.sound.name.retro');
      case 'coin':
        return m('settings.notifications.sound.name.coin');
      case 'powerup':
        return m('settings.notifications.sound.name.powerup');
      case 'fanfare':
        return m('settings.notifications.sound.name.fanfare');
      case 'laser':
        return m('settings.notifications.sound.name.laser');
      case 'robot':
        return m('settings.notifications.sound.name.robot');
      case 'ufo':
        return m('settings.notifications.sound.name.ufo');
      case 'beepboop':
        return m('settings.notifications.sound.name.beepboop');
      case 'dialup':
        return m('settings.notifications.sound.name.dialup');
      case 'r2d2':
        return m('settings.notifications.sound.name.r2d2');
      case 'harp':
        return m('settings.notifications.sound.name.harp');
      case 'music-box':
        return m('settings.notifications.sound.name.music_box');
      case 'celesta':
        return m('settings.notifications.sound.name.celesta');
      case 'synth':
        return m('settings.notifications.sound.name.synth');
      case 'orchestra':
        return m('settings.notifications.sound.name.orchestra');
      case 'la-cucaracha':
        return m('settings.notifications.sound.name.la_cucaracha');
      case 'chaos':
        return m('settings.notifications.sound.name.chaos');
      case 'glitch':
        return m('settings.notifications.sound.name.glitch');
      case 'siren':
        return m('settings.notifications.sound.name.siren');
      case 'dubstep':
        return m('settings.notifications.sound.name.dubstep');
      case 'circus':
        return m('settings.notifications.sound.name.circus');
    }
  }

  // Push notifications state
  let pushEnabled = $derived(serverInfo.pushNotificationsEnabled);
  let showPushControls = $derived(isBrowserWebPushRuntime() && pushEnabled);
  const pushCapability = getPushCapability();
  const pushSupported = pushCapability === 'supported';
  const needsIosHomeScreen = pushCapability === 'ios_home_screen_required';
  let pushPermission = $state<NotificationPermission | null>(getPermission());
  let pushSubscribed = $state(false);
  let pushLoading = $state(false);
  let pushError = $state<string | null>(null);
  let pushTestLoading = $state(false);
  let pushTestStatus = $state<'sent' | 'failed' | null>(null);
  let pushSubscriptionGeneration = 0;
  let pushEnableGeneration = 0;
  let pushTestGeneration = 0;

  // Check push subscription status on mount
  $effect(() => {
    const serverId = activeServerId;
    const generation = ++pushSubscriptionGeneration;
    ++pushEnableGeneration;
    ++pushTestGeneration;
    pushSubscribed = false;
    pushLoading = false;
    pushError = null;
    pushTestLoading = false;
    pushTestStatus = null;
    if (showPushControls && pushSupported) {
      pushPermission = getPermission();
      checkPushSubscription(serverId).then((subscribed) => {
        if (activeServerId === serverId && pushSubscriptionGeneration === generation) {
          pushSubscribed = subscribed;
        }
      });
    }
  });

  async function handleEnablePush() {
    const serverId = activeServerId;
    const generation = ++pushEnableGeneration;
    const vapidKey = serverInfo.vapidPublicKey;
    if (!vapidKey) {
      pushError = m('settings.notifications.push.not_configured');
      return;
    }

    pushLoading = true;
    pushError = null;

    try {
      const success = await ensureRegistered(serverId, vapidKey, { prompt: true });
      if (activeServerId !== serverId || pushEnableGeneration !== generation) return;
      pushPermission = getPermission();
      if (success) {
        pushSubscribed = true;
        void refreshPushSubscriptions().catch(() => undefined);
      } else {
        pushError =
          pushPermission === 'denied'
            ? m('settings.notifications.push.blocked_error')
            : m('settings.notifications.push.enable_failed');
      }
    } catch {
      if (activeServerId === serverId && pushEnableGeneration === generation) {
        pushError = m('settings.notifications.push.enable_error');
      }
    } finally {
      if (activeServerId === serverId && pushEnableGeneration === generation) pushLoading = false;
    }
  }

  async function handleTestPush() {
    const serverId = activeServerId;
    const generation = ++pushTestGeneration;
    pushTestLoading = true;
    pushTestStatus = null;
    try {
      const sent = await sendTestNotification(serverId);
      if (activeServerId === serverId && pushTestGeneration === generation) {
        pushTestStatus = sent ? 'sent' : 'failed';
      }
    } catch {
      if (activeServerId === serverId && pushTestGeneration === generation) {
        pushTestStatus = 'failed';
      }
    } finally {
      if (activeServerId === serverId && pushTestGeneration === generation) {
        pushTestLoading = false;
      }
    }
  }
</script>

<PaneHeader
  title={m('settings.notifications.title')}
  subtitle={m('settings.notifications.subtitle')}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <NotificationPolicySettings />

  <!-- Push Notifications Section (only show if enabled on server) -->
  {#if showPushControls}
    <div class="max-w-lg">
      <h3 class="mb-4 text-sm font-semibold text-muted">
        {m('settings.notifications.push.title')}
      </h3>

      {#if needsIosHomeScreen}
        <Hint tone="info">
          <div>
            <p class="font-medium">{m('settings.notifications.push.ios_home_screen_title')}</p>
            <p class="mt-1 text-sm text-muted">
              {m('settings.notifications.push.ios_home_screen_description')}
            </p>
          </div>
        </Hint>
      {:else if !pushSupported}
        <div class="surface-box px-4 py-3 text-sm text-muted">
          {m('settings.notifications.push.not_supported')}
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
              {m('settings.notifications.push.blocked_title')}
            </p>
            <p class="mt-1 text-sm text-muted">
              {m('settings.notifications.push.blocked_description')}
            </p>
          </div>
        {:else if pushSubscribed}
          <div class="flex flex-col gap-3">
            <Hint tone="success">
              <div>
                <p class="font-medium">{m('settings.notifications.push.enabled_title')}</p>
                <p class="mt-1 text-sm text-muted">
                  {m('settings.notifications.push.enabled_description')}
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
                loadingText={m('settings.notifications.push.testing')}
              >
                {m('settings.notifications.push.test_button')}
              </Button>
              {#if pushTestStatus === 'sent'}
                <span class="text-sm text-success" role="status">
                  {m('settings.notifications.push.test_sent')}
                </span>
              {:else if pushTestStatus === 'failed'}
                <span class="text-sm text-danger" role="alert">
                  {m('settings.notifications.push.test_failed')}
                </span>
              {/if}
            </div>
          </div>
        {:else}
          <div class="flex items-center justify-between surface-box px-4 py-3">
            <div>
              <p class="font-medium">{m('settings.notifications.push.enable_title')}</p>
              <p class="mt-1 text-sm text-muted">
                {m('settings.notifications.push.enable_description')}
              </p>
            </div>
            <Button
              variant="action"
              size="sm"
              onclick={handleEnablePush}
              disabled={pushLoading}
              loading={pushLoading}
              loadingText={m('settings.notifications.push.enabling')}
            >
              {m('settings.notifications.push.enable_button')}
            </Button>
          </div>
        {/if}
      {/if}
    </div>
  {/if}

  <!-- Notification Sound Section -->
  <div class="max-w-lg">
    <h3 class="mb-4 text-sm font-semibold text-muted">
      {m('settings.notifications.sound.title')}
    </h3>

    <div class="flex flex-col gap-4">
      {#each soundCategories as category (category)}
        {@const sounds = getSoundsForCategory(category)}
        <div>
          <h4 class="mb-2 text-xs font-medium tracking-wide text-muted uppercase">
            {soundCategoryLabel(category)}
          </h4>
          <div
            class="flex flex-col gap-1"
            role="radiogroup"
            aria-label={soundCategoryLabel(category)}
          >
            {#each sounds as sound (sound.id)}
              {@const isSelected = userPreferences.notificationSound === sound.id}
              <ChoiceRow
                label={soundNameLabel(sound.id)}
                selected={isSelected}
                onclick={() => selectSound(sound.id)}
              />
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>

  <FormSection title={m('settings.notifications.sound.shape_title')} maxWidth="max-w-lg" bordered>
    {#snippet actions()}
      <Button
        variant="secondary"
        size="sm"
        onclick={previewSelectedSound}
        disabled={userPreferences.notificationSound === 'silent'}
      >
        {m('settings.notifications.sound.preview')}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        onclick={() => userPreferences.resetNotificationSoundFilters()}
      >
        {m('settings.notifications.sound.reset')}
      </Button>
    {/snippet}

    <div class="flex flex-col gap-2">
      <RangeField
        id="notification-volume-filter"
        testid="notification-volume-filter"
        label={m('settings.notifications.sound.volume')}
        icon="icon-[uil--volume]"
        min={0}
        max={2}
        step={0.05}
        value={userPreferences.notificationSoundFilters.volume}
        displayValue={formatVolume(userPreferences.notificationSoundFilters.volume)}
        oninput={(event) => updateSoundFilter('volume', event)}
        onchange={previewSelectedSound}
      />

      <RangeField
        id="notification-high-pass-filter"
        testid="notification-high-pass-filter"
        label={m('settings.notifications.sound.tinny')}
        icon="icon-[uil--bolt]"
        min={20}
        max={2000}
        step={10}
        value={userPreferences.notificationSoundFilters.highPassHz}
        displayValue={formatTinny(userPreferences.notificationSoundFilters.highPassHz)}
        oninput={(event) => updateSoundFilter('highPassHz', event)}
        onchange={previewSelectedSound}
      />

      <RangeField
        id="notification-low-pass-filter"
        testid="notification-low-pass-filter"
        label={m('settings.notifications.sound.muffled')}
        icon="icon-[uil--volume-mute]"
        min={0}
        max={100}
        value={muffledAmountFromLowPassHz(userPreferences.notificationSoundFilters.lowPassHz)}
        displayValue={formatMuffled(userPreferences.notificationSoundFilters.lowPassHz)}
        oninput={updateMuffledFilter}
        onchange={previewSelectedSound}
      />

      <RangeField
        id="notification-echo-filter"
        testid="notification-echo-filter"
        label={m('settings.notifications.sound.echo')}
        icon="icon-[uil--redo]"
        min={0}
        max={100}
        value={userPreferences.notificationSoundFilters.echo}
        displayValue={formatEffect(userPreferences.notificationSoundFilters.echo)}
        oninput={(event) => updateSoundFilter('echo', event)}
        onchange={previewSelectedSound}
      />

      <RangeField
        id="notification-reverb-filter"
        testid="notification-reverb-filter"
        label={m('settings.notifications.sound.reverb')}
        icon="icon-[uil--cloud]"
        min={0}
        max={100}
        value={userPreferences.notificationSoundFilters.reverb}
        displayValue={formatEffect(userPreferences.notificationSoundFilters.reverb)}
        oninput={(event) => updateSoundFilter('reverb', event)}
        onchange={previewSelectedSound}
      />

      <RangeField
        id="notification-crunch-filter"
        testid="notification-crunch-filter"
        label={m('settings.notifications.sound.crunch')}
        icon="icon-[uil--fire]"
        min={0}
        max={100}
        value={userPreferences.notificationSoundFilters.crunch}
        displayValue={formatEffect(userPreferences.notificationSoundFilters.crunch)}
        oninput={(event) => updateSoundFilter('crunch', event)}
        onchange={previewSelectedSound}
      />
    </div>
  </FormSection>
</div>
