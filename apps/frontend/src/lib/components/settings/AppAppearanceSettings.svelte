<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import { m } from '$lib/i18n/messages';
  import { userPreferences, type DisplayTheme } from '$lib/state/userPreferences.svelte';
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
</script>

<PageTitle title={m('settings.app_preferences.appearance.title')} />
<PaneHeader
  title={m('settings.app_preferences.appearance.title')}
  subtitle={m('settings.app_preferences.subtitle')}
/>

<PaneContent>
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
</PaneContent>
