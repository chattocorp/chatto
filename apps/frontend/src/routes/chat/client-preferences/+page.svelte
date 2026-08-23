<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { localeDisplayName, selectableLocales } from '$lib/i18n/locales';
  import { getLocale, setLocale, type Locale } from '$lib/i18n/runtime';
  import {
    userPreferences,
    type ComposerEditorKind,
    type ComposerSendMode,
    type DisplayTheme
  } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, FormSection, PaneHeader } from '$lib/ui';

  const activeLocale = $derived(getLocale());

  function handleLocaleSelect(locale: Locale) {
    if (locale === activeLocale) return;
    void setLocale(locale);
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

  const languageOptions = $derived(
    selectableLocales.map((locale) => ({
      value: locale,
      label: localeDisplayName(locale, activeLocale)
    }))
  );

  const editorOptions = $derived([
    {
      value: 'visual',
      label: m('settings.preferences.editor.visual.label'),
      description: m('settings.preferences.editor.visual.description')
    },
    {
      value: 'markdown',
      label: m('settings.preferences.editor.markdown.label'),
      description: m('settings.preferences.editor.markdown.description')
    }
  ] satisfies Array<{ value: ComposerEditorKind; label: string; description: string }>);

  const sendModeOptions = $derived([
    {
      value: 'enter',
      label: m('settings.preferences.send_mode.enter.label'),
      description: m('settings.preferences.send_mode.enter.description')
    },
    {
      value: 'modifier-enter',
      label: m('settings.preferences.send_mode.modifier_enter.label'),
      description: m('settings.preferences.send_mode.modifier_enter.description')
    }
  ] satisfies Array<{ value: ComposerSendMode; label: string; description: string }>);
</script>

<div class="flex h-full min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m('settings.client_preferences.title')}
    subtitle={m('settings.client_preferences.subtitle')}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <FormSection title={m('settings.preferences.theme.title')} maxWidth="max-w-md">
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
    </FormSection>

    <FormSection title={m('settings.preferences.editor.title')} maxWidth="max-w-md" bordered>
      <p class="mb-3 text-sm text-muted">{m('settings.preferences.browser_scope')}</p>
      <div
        class="flex flex-col gap-2"
        role="radiogroup"
        aria-label={m('settings.preferences.editor.title')}
      >
        {#each editorOptions as option (option.value)}
          <ChoiceRow
            label={option.label}
            description={option.description}
            selected={userPreferences.composerEditor === option.value}
            onclick={() => (userPreferences.composerEditor = option.value)}
          />
        {/each}
      </div>
    </FormSection>

    <FormSection title={m('settings.preferences.send_mode.title')} maxWidth="max-w-md" bordered>
      <p class="mb-3 text-sm text-muted">{m('settings.preferences.browser_scope')}</p>
      <div
        class="flex flex-col gap-2"
        role="radiogroup"
        aria-label={m('settings.preferences.send_mode.title')}
      >
        {#each sendModeOptions as option (option.value)}
          <ChoiceRow
            label={option.label}
            description={option.description}
            selected={userPreferences.composerSendMode === option.value}
            onclick={() => (userPreferences.composerSendMode = option.value)}
          />
        {/each}
      </div>
    </FormSection>

    <FormSection title={m('settings.preferences.language.title')} maxWidth="max-w-md" bordered>
      <p class="mb-3 text-sm text-muted">
        {m('settings.preferences.language.description')}
      </p>

      <div
        class="flex flex-col gap-2"
        role="radiogroup"
        aria-label={m('settings.preferences.language.title')}
      >
        {#each languageOptions as option (option.value)}
          <ChoiceRow
            label={option.label}
            selected={activeLocale === option.value}
            onclick={() => handleLocaleSelect(option.value)}
          />
        {/each}
      </div>
    </FormSection>
  </div>
</div>
