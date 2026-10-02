<script lang="ts">
  import { Panel, ChoiceRow, FormSection, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Button, RangeField } from '$lib/ui/form';
  import NotificationPolicySettings from '$lib/components/settings/NotificationPolicySettings.svelte';
  import PushDeviceSettings from '$lib/components/settings/PushDeviceSettings.svelte';
  import { getServerNotificationPreferences } from '$lib/state/serverNotificationPreferences.svelte';
  import {
    notificationSounds,
    playNotificationSound,
    soundCategories,
    type NotificationSoundFilters,
    type NotificationSoundId,
    type SoundCategory
  } from '$lib/audio/notificationSounds';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const serverId = serverScope.serverId;
  const notificationPreferences = getServerNotificationPreferences(serverId);

  function selectSound(soundId: NotificationSoundId) {
    notificationPreferences.notificationSound = soundId;
    if (soundId !== 'silent') {
      playNotificationSound(soundId, notificationPreferences.notificationSoundFilters);
    }
  }

  function previewSelectedSound() {
    if (notificationPreferences.notificationSound === 'silent') return;
    playNotificationSound(
      notificationPreferences.notificationSound,
      notificationPreferences.notificationSoundFilters
    );
  }

  function updateSoundFilter(key: keyof NotificationSoundFilters, event: Event) {
    const value = Number((event.currentTarget as HTMLInputElement).value);
    notificationPreferences.setNotificationSoundFilter(key, value);
  }

  function updateMuffledFilter(event: Event) {
    const amount = Number((event.currentTarget as HTMLInputElement).value);
    notificationPreferences.setNotificationSoundFilter(
      'lowPassHz',
      lowPassHzFromMuffledAmount(amount)
    );
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
</script>

<PageTitle title={m('settings.notifications.title')} />

<div class="pane-page">
  <PaneHeader
    title={m('settings.notifications.title')}
    subtitle={m('settings.notifications.subtitle')}
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      <PushDeviceSettings />

      <NotificationPolicySettings />

      <!-- Notification Sound Section -->
      <Panel title={m('settings.notifications.sound.title')} icon="iconify icon-[uil--volume]">
        <div class="flex max-w-lg flex-col gap-4">
          {#each soundCategories as category (category)}
            {@const sounds = getSoundsForCategory(category)}
            <FormSection title={soundCategoryLabel(category)}>
              <div
                class="flex flex-col gap-1"
                role="radiogroup"
                aria-label={soundCategoryLabel(category)}
              >
                {#each sounds as sound (sound.id)}
                  {@const isSelected = notificationPreferences.notificationSound === sound.id}
                  <ChoiceRow
                    label={soundNameLabel(sound.id)}
                    selected={isSelected}
                    onclick={() => selectSound(sound.id)}
                  />
                {/each}
              </div>
            </FormSection>
          {/each}
        </div>
      </Panel>

      <Panel
        title={m('settings.notifications.sound.shape_title')}
        icon="iconify icon-[uil--sliders-v-alt]"
      >
        {#snippet actions()}
          <Button
            variant="secondary"
            size="sm"
            onclick={previewSelectedSound}
            disabled={notificationPreferences.notificationSound === 'silent'}
          >
            {m('settings.notifications.sound.preview')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onclick={() => notificationPreferences.resetNotificationSoundFilters()}
          >
            {m('settings.notifications.sound.reset')}
          </Button>
        {/snippet}

        <div class="flex max-w-lg flex-col gap-2">
          <RangeField
            id="notification-volume-filter"
            testid="notification-volume-filter"
            label={m('settings.notifications.sound.volume')}
            icon="icon-[uil--volume]"
            min={0}
            max={2}
            step={0.05}
            value={notificationPreferences.notificationSoundFilters.volume}
            displayValue={formatVolume(notificationPreferences.notificationSoundFilters.volume)}
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
            value={notificationPreferences.notificationSoundFilters.highPassHz}
            displayValue={formatTinny(notificationPreferences.notificationSoundFilters.highPassHz)}
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
            value={muffledAmountFromLowPassHz(
              notificationPreferences.notificationSoundFilters.lowPassHz
            )}
            displayValue={formatMuffled(notificationPreferences.notificationSoundFilters.lowPassHz)}
            oninput={updateMuffledFilter}
            onchange={previewSelectedSound}
          />

          <RangeField
            id="notification-echo-filter"
            testid="notification-echo-filter"
            label={m('settings.notifications.sound.echo')}
            icon="icon-[uil--redo] rtl:-scale-x-100"
            min={0}
            max={100}
            value={notificationPreferences.notificationSoundFilters.echo}
            displayValue={formatEffect(notificationPreferences.notificationSoundFilters.echo)}
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
            value={notificationPreferences.notificationSoundFilters.reverb}
            displayValue={formatEffect(notificationPreferences.notificationSoundFilters.reverb)}
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
            value={notificationPreferences.notificationSoundFilters.crunch}
            displayValue={formatEffect(notificationPreferences.notificationSoundFilters.crunch)}
            oninput={(event) => updateSoundFilter('crunch', event)}
            onchange={previewSelectedSound}
          />
        </div>
      </Panel>
    </div>
  </PaneContent>
</div>
