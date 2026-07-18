<script lang="ts">
  import { ChoiceRow, FormSection } from '$lib/ui';
  import { Button, RangeField } from '$lib/ui/form';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import {
    notificationSounds,
    playNotificationSound,
    soundCategories,
    type NotificationSoundFilters,
    type NotificationSoundId,
    type SoundCategory
  } from '$lib/audio/notificationSounds';
  import * as m from '$lib/i18n/messages';

  function selectSound(soundId: NotificationSoundId) {
    userPreferences.notificationSound = soundId;
    if (soundId !== 'silent') previewSelectedSound();
  }

  function previewSelectedSound() {
    if (userPreferences.notificationSound === 'silent') return;
    playNotificationSound(
      userPreferences.notificationSound,
      userPreferences.notificationSoundFilters
    );
  }

  function updateSoundFilter(key: keyof NotificationSoundFilters, event: Event) {
    userPreferences.setNotificationSoundFilter(
      key,
      Number((event.currentTarget as HTMLInputElement).value)
    );
  }

  function updateMuffledFilter(event: Event) {
    const amount = Number((event.currentTarget as HTMLInputElement).value);
    userPreferences.setNotificationSoundFilter('lowPassHz', 20000 - (amount / 100) * 19200);
  }

  function muffledAmount(value: number) {
    return Math.round(((20000 - value) / 19200) * 100);
  }

  function formatEffect(value: number) {
    return value <= 0 ? m['settings.notifications.sound.off']() : `${Math.round(value)}%`;
  }

  function soundCategoryLabel(category: SoundCategory) {
    const labels = {
      Silent: m['settings.notifications.sound.category.silent'](),
      Simple: m['settings.notifications.sound.category.simple'](),
      Playful: m['settings.notifications.sound.category.playful'](),
      Robots: m['settings.notifications.sound.category.robots'](),
      Musical: m['settings.notifications.sound.category.musical'](),
      'Here Be Dragons': m['settings.notifications.sound.category.here_be_dragons']()
    } satisfies Record<SoundCategory, string>;
    return labels[category];
  }

  function soundNameLabel(soundId: NotificationSoundId) {
    const labels = {
      silent: m['settings.notifications.sound.name.silent'](),
      ding: m['settings.notifications.sound.name.ding'](),
      'chime-up': m['settings.notifications.sound.name.chime_up'](),
      'chime-down': m['settings.notifications.sound.name.chime_down'](),
      pop: m['settings.notifications.sound.name.pop'](),
      bubble: m['settings.notifications.sound.name.bubble'](),
      retro: m['settings.notifications.sound.name.retro'](),
      coin: m['settings.notifications.sound.name.coin'](),
      powerup: m['settings.notifications.sound.name.powerup'](),
      fanfare: m['settings.notifications.sound.name.fanfare'](),
      laser: m['settings.notifications.sound.name.laser'](),
      robot: m['settings.notifications.sound.name.robot'](),
      ufo: m['settings.notifications.sound.name.ufo'](),
      beepboop: m['settings.notifications.sound.name.beepboop'](),
      dialup: m['settings.notifications.sound.name.dialup'](),
      r2d2: m['settings.notifications.sound.name.r2d2'](),
      harp: m['settings.notifications.sound.name.harp'](),
      'music-box': m['settings.notifications.sound.name.music_box'](),
      celesta: m['settings.notifications.sound.name.celesta'](),
      synth: m['settings.notifications.sound.name.synth'](),
      orchestra: m['settings.notifications.sound.name.orchestra'](),
      'la-cucaracha': m['settings.notifications.sound.name.la_cucaracha'](),
      chaos: m['settings.notifications.sound.name.chaos'](),
      glitch: m['settings.notifications.sound.name.glitch'](),
      siren: m['settings.notifications.sound.name.siren'](),
      dubstep: m['settings.notifications.sound.name.dubstep'](),
      circus: m['settings.notifications.sound.name.circus']()
    } satisfies Record<NotificationSoundId, string>;
    return labels[soundId];
  }
</script>

<FormSection title={m['settings.notifications.sound.title']()} maxWidth="max-w-lg">
  <div class="flex flex-col gap-4">
    {#each soundCategories as category (category)}
      {@const sounds = notificationSounds.filter((sound) => sound.category === category)}
      <div>
        <h3 class="mb-2 text-xs font-medium tracking-wide text-muted uppercase">
          {soundCategoryLabel(category)}
        </h3>
        <div
          class="flex flex-col gap-1"
          role="radiogroup"
          aria-label={soundCategoryLabel(category)}
        >
          {#each sounds as sound (sound.id)}
            <ChoiceRow
              label={soundNameLabel(sound.id)}
              selected={userPreferences.notificationSound === sound.id}
              onclick={() => selectSound(sound.id)}
            />
          {/each}
        </div>
      </div>
    {/each}
  </div>
</FormSection>

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
    <RangeField
      id="notification-volume-filter"
      testid="notification-volume-filter"
      label={m['settings.notifications.sound.volume']()}
      icon="uil--volume"
      min={0}
      max={2}
      step={0.05}
      value={userPreferences.notificationSoundFilters.volume}
      displayValue={`${Math.round(userPreferences.notificationSoundFilters.volume * 100)}%`}
      oninput={(event) => updateSoundFilter('volume', event)}
      onchange={previewSelectedSound}
    />
    <RangeField
      id="notification-high-pass-filter"
      testid="notification-high-pass-filter"
      label={m['settings.notifications.sound.tinny']()}
      icon="uil--bolt"
      min={20}
      max={2000}
      step={10}
      value={userPreferences.notificationSoundFilters.highPassHz}
      displayValue={userPreferences.notificationSoundFilters.highPassHz <= 20
        ? m['settings.notifications.sound.off']()
        : `${Math.round(((userPreferences.notificationSoundFilters.highPassHz - 20) / 1980) * 100)}%`}
      oninput={(event) => updateSoundFilter('highPassHz', event)}
      onchange={previewSelectedSound}
    />
    <RangeField
      id="notification-low-pass-filter"
      testid="notification-low-pass-filter"
      label={m['settings.notifications.sound.muffled']()}
      icon="uil--volume-mute"
      min={0}
      max={100}
      value={muffledAmount(userPreferences.notificationSoundFilters.lowPassHz)}
      displayValue={formatEffect(muffledAmount(userPreferences.notificationSoundFilters.lowPassHz))}
      oninput={updateMuffledFilter}
      onchange={previewSelectedSound}
    />
    {#each [['echo', 'uil--redo', m['settings.notifications.sound.echo']()], ['reverb', 'uil--cloud', m['settings.notifications.sound.reverb']()], ['crunch', 'uil--fire', m['settings.notifications.sound.crunch']()]] as [filter, icon, label] (filter)}
      <RangeField
        id={`notification-${filter}-filter`}
        testid={`notification-${filter}-filter`}
        {label}
        {icon}
        min={0}
        max={100}
        value={userPreferences.notificationSoundFilters[filter as keyof NotificationSoundFilters]}
        displayValue={formatEffect(
          userPreferences.notificationSoundFilters[filter as keyof NotificationSoundFilters]
        )}
        oninput={(event) => updateSoundFilter(filter as keyof NotificationSoundFilters, event)}
        onchange={previewSelectedSound}
      />
    {/each}
  </div>
</FormSection>
