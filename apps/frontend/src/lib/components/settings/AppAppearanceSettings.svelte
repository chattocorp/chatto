<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    userPreferences,
    type DisplayTheme,
    type ThreadPanePresentation
  } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, PageTitle, PaneContent, PaneHeader } from '$lib/ui';

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
        <p class="mb-3 text-sm text-muted">{m('settings.preferences.browser_scope')}</p>
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

    <Panel
      title={m('settings.preferences.thread_pane.title')}
      icon="iconify icon-[uil--window-section]"
    >
      <div class="max-w-md">
        <p class="mb-3 text-sm text-muted">{m('settings.preferences.browser_scope')}</p>
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
