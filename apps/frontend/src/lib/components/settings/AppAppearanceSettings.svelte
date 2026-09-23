<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    userPreferences,
    type DisplayTheme,
    type SurfaceDepth,
    type ThreadPanePresentation
  } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import AccentColorPicker from './AccentColorPicker.svelte';
  import RangeField from '$lib/ui/form/RangeField.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';

  const contrastDisplayValue = $derived(`${Math.round((userPreferences.contrastAge - 20) * 5)}%`);
  const contrastValueText = $derived(
    `${contrastDisplayValue}, ${m(
      userPreferences.contrastAge < 30
        ? 'settings.preferences.contrast.softer'
        : userPreferences.contrastAge > 30
          ? 'settings.preferences.contrast.stronger'
          : 'settings.preferences.contrast.current'
    )}`
  );
  let cacheCleared = $state(false);

  async function clearDeviceCache() {
    await serverRegistry.clearDeviceSavedViews();
    cacheCleared = true;
  }

  const themeOptions = $derived([
    {
      value: 'system',
      label: m('settings.preferences.theme.system.label'),
      description: m('settings.preferences.theme.system.description')
    },
    {
      value: 'light',
      label: m('settings.preferences.theme.light.label'),
      description: m('settings.preferences.theme.light.description')
    },
    {
      value: 'dark',
      label: m('settings.preferences.theme.dark.label'),
      description: m('settings.preferences.theme.dark.description')
    }
  ] satisfies Array<{
    value: DisplayTheme;
    label: string;
    description: string;
  }>);

  const depthOptions = $derived([
    { value: 'flat', label: m('settings.preferences.depth.flat') },
    { value: '3d', label: m('settings.preferences.depth.three_d') },
    { value: 'very-3d', label: m('settings.preferences.depth.very_3d') }
  ] satisfies Array<{ value: SurfaceDepth; label: string }>);

  const threadPaneOptions = $derived([
    {
      value: 'overlay',
      label: m('settings.preferences.thread_pane.overlay.label'),
      description: m('settings.preferences.thread_pane.overlay.description')
    },
    {
      value: 'split',
      label: m('settings.preferences.thread_pane.split.label'),
      description: m('settings.preferences.thread_pane.split.description')
    }
  ] satisfies Array<{
    value: ThreadPanePresentation;
    label: string;
    description: string;
  }>);
</script>

<PageTitle title={m('settings.app_preferences.appearance.title')} />
<PaneHeader
  title={m('settings.app_preferences.appearance.title')}
  subtitle={m('settings.app_preferences.subtitle')}
/>

<PaneContent>
  <div class="flex flex-col gap-6">
    <Panel title={m('settings.preferences.theme.title')} icon="iconify icon-[uil--palette]">
      <div class="max-w-md">
        <div
          class="flex flex-col gap-2"
          role="radiogroup"
          aria-label={m('settings.preferences.theme.title')}
        >
          {#each themeOptions as option (option.value)}
            <ChoiceRow
              label={option.label}
              description={option.description}
              selected={userPreferences.displayTheme === option.value}
              onclick={() => (userPreferences.displayTheme = option.value)}
            />
          {/each}
        </div>
      </div>
    </Panel>

    <Panel title={m('settings.preferences.accent.title')} icon="iconify icon-[uil--palette]">
      <AccentColorPicker
        value={userPreferences.accentColor}
        onchange={(value) => (userPreferences.accentColor = value)}
      />
    </Panel>

    <Panel title={m('settings.preferences.depth.title')} icon="iconify icon-[uil--layer-group]">
      <div class="flex flex-col gap-6">
        <div class="flex max-w-md flex-col gap-2" role="radiogroup" aria-label={m('settings.preferences.depth.title')}>
          {#each depthOptions as option (option.value)}
            <ChoiceRow
              label={option.label}
              selected={userPreferences.surfaceDepth === option.value}
              onclick={() => (userPreferences.surfaceDepth = option.value)}
            />
          {/each}
        </div>
        <RangeField
          id="ui-contrast"
          label={m('settings.preferences.contrast.label')}
          min={20}
          max={40}
          step={0.5}
          ticks={[20, 30, 40]}
          value={userPreferences.contrastAge}
          displayValue={contrastDisplayValue}
          ariaValueText={contrastValueText}
          prominent
          oninput={(event) =>
            (userPreferences.contrastAge = (event.currentTarget as HTMLInputElement).valueAsNumber)}
        />
      </div>
    </Panel>

    <Panel
      title={m('settings.preferences.thread_pane.title')}
      icon="iconify icon-[uil--window-section]"
    >
      <div class="max-w-md">
        <div
          class="flex flex-col gap-2"
          role="radiogroup"
          aria-label={m('settings.preferences.thread_pane.title')}
        >
          {#each threadPaneOptions as option (option.value)}
            <ChoiceRow
              label={option.label}
              description={option.description}
              selected={userPreferences.threadPanePresentation === option.value}
              onclick={() => (userPreferences.threadPanePresentation = option.value)}
            />
          {/each}
        </div>
      </div>
    </Panel>
    <Panel title={m('ui.saved_view.cache_title')} icon="iconify icon-[uil--database]">
      <p class="mb-3 text-muted">{m('ui.saved_view.cache_description')}</p>
      <button class="button" type="button" onclick={clearDeviceCache}>
        {m('ui.saved_view.clear_cache')}
      </button>
      {#if cacheCleared}<p role="status">{m('ui.saved_view.cache_cleared')}</p>{/if}
    </Panel>
  </div>
</PaneContent>
