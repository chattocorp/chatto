<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    userPreferences,
    contrastAgeStep,
    surfaceDepthStep,
    type DisplayTheme,
    type ThreadPanePresentation
  } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, FormSection, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import AccentColorPicker from './AccentColorPicker.svelte';
  import SurfaceTonePicker from './SurfaceTonePicker.svelte';
  import RangeField from '$lib/ui/form/RangeField.svelte';

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
  const contrastTicks = Array.from({ length: 11 }, (_, index) => 20 + index * contrastAgeStep);
  const depthTicks = Array.from({ length: 11 }, (_, index) => index * surfaceDepthStep);
  const depthDisplayValue = $derived(`${userPreferences.surfaceDepth}%`);
  // Name the former modes where the slider matches them exactly.
  const depthName = $derived(
    userPreferences.surfaceDepth === 0
      ? m('settings.preferences.depth.flat')
      : userPreferences.surfaceDepth === 50
        ? m('settings.preferences.depth.three_d')
        : userPreferences.surfaceDepth === 100
          ? m('settings.preferences.depth.very_3d')
          : undefined
  );
  const depthValueText = $derived(
    depthName ? `${depthDisplayValue}, ${depthName}` : depthDisplayValue
  );
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

    <Panel title={m('settings.preferences.customization.title')} icon="iconify icon-[uil--palette]">
      <div class="flex flex-col gap-6">
        <FormSection title={m('settings.preferences.accent.title')}>
          <AccentColorPicker
            value={userPreferences.accentColor}
            onchange={(value) => (userPreferences.accentColor = value)}
          />
        </FormSection>
        <FormSection title={m('settings.preferences.theme.title')}>
          {#if userPreferences.effectiveDisplayTheme === 'dark'}
            <SurfaceTonePicker
              theme="dark"
              value={userPreferences.darkSurfaceTone}
              onchange={(value) => (userPreferences.darkSurfaceTone = value)}
            />
          {:else}
            <SurfaceTonePicker
              theme="light"
              value={userPreferences.lightSurfaceTone}
              onchange={(value) => (userPreferences.lightSurfaceTone = value)}
            />
          {/if}
        </FormSection>
        <FormSection title={m('settings.preferences.depth.title')}>
          <div class="@container">
            <div class="grid gap-3 @min-[36rem]:grid-cols-2">
              <RangeField
                id="ui-depth"
                label={m('settings.preferences.depth.label')}
                min={0}
                max={100}
                step={surfaceDepthStep}
                ticks={depthTicks}
                value={userPreferences.surfaceDepth}
                displayValue={depthDisplayValue}
                ariaValueText={depthValueText}
                oninput={(event) =>
                  (userPreferences.surfaceDepth = (
                    event.currentTarget as HTMLInputElement
                  ).valueAsNumber)}
              />
              <RangeField
                id="ui-contrast"
                label={m('settings.preferences.contrast.label')}
                min={20}
                max={40}
                step={contrastAgeStep}
                ticks={contrastTicks}
                value={userPreferences.contrastAge}
                displayValue={contrastDisplayValue}
                ariaValueText={contrastValueText}
                oninput={(event) =>
                  (userPreferences.contrastAge = (
                    event.currentTarget as HTMLInputElement
                  ).valueAsNumber)}
              />
            </div>
          </div>
        </FormSection>
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
  </div>
</PaneContent>
