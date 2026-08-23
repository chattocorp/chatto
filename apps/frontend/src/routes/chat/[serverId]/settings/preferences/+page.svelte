<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { localeDisplayName, selectableLocales } from '$lib/i18n/locales';
  import { getLocale, setLocale, type Locale } from '$lib/i18n/runtime';
  import {
    userPreferences,
    type ComposerEditorKind,
    type ComposerSendMode,
    type DisplayTheme,
    type TimeFormatPreference
  } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, PaneHeader, FormSection } from '$lib/ui';
  import { Button, Combobox } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatMessageTime, hour12ForTimeFormat } from '$lib/utils/formatTime';

  const activeLocale = $derived(getLocale());

  // All available IANA timezone names
  const allTimezones = Intl.supportedValuesOf('timeZone');

  // These writable derived values are edit buffers. Saving commits all time
  // preferences together to the client-wide preference store.
  let timezoneSearch = $derived(userPreferences.timezone ?? '');
  let selectedTimezone = $derived(userPreferences.timezone ?? '');
  let selectedTimeFormat = $derived<TimeFormatPreference>(userPreferences.timeFormat);

  // Filter timezone list based on search input
  let filteredTimezones = $derived(
    timezoneSearch
      ? allTimezones.filter((tz) => tz.toLowerCase().includes(timezoneSearch.toLowerCase()))
      : allTimezones
  );

  // Cap displayed results to avoid rendering 400+ items
  let displayedTimezones = $derived(filteredTimezones.slice(0, 50));

  // Track if the form has been modified
  const isModified = $derived(
    (selectedTimezone || null) !== userPreferences.timezone ||
      selectedTimeFormat !== userPreferences.timeFormat
  );

  // Timezone validation
  const timezoneError = $derived.by(() => {
    if (!timezoneSearch) return undefined;
    if (allTimezones.includes(timezoneSearch)) return undefined;
    return m('settings.preferences.timezone.invalid');
  });

  const selectedTimezoneTime = $derived.by(() => {
    if (!selectedTimezone) return null;

    return formatMessageTime(
      new Date(),
      {
        effectiveTimezone: selectedTimezone,
        effectiveHour12: hour12ForTimeFormat(selectedTimeFormat)
      },
      activeLocale
    );
  });

  function handleTimezoneTextChange(text: string) {
    if (!text || allTimezones.includes(text)) selectedTimezone = text;
  }

  function handleLocaleSelect(locale: Locale) {
    if (locale === activeLocale) return;
    void setLocale(locale);
  }

  function handleSave() {
    // Validate timezone if set
    if (timezoneSearch && !allTimezones.includes(timezoneSearch)) return;

    userPreferences.setTimePreferences({
      timezone: selectedTimezone || null,
      timeFormat: selectedTimeFormat
    });
    toast.success(m('settings.preferences.saved'));
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

  const timeFormatOptions = $derived([
    {
      value: 'auto',
      label: m('settings.preferences.time_format.browser_default.label'),
      description: m('settings.preferences.time_format.browser_default.description')
    },
    {
      value: '12h',
      label: m('settings.preferences.time_format.12h.label'),
      description: m('settings.preferences.time_format.12h.description')
    },
    {
      value: '24h',
      label: m('settings.preferences.time_format.24h.label'),
      description: m('settings.preferences.time_format.24h.description')
    }
  ] satisfies Array<{
    value: TimeFormatPreference;
    label: string;
    description: string;
  }>);
</script>

<PaneHeader
  title={m('settings.preferences.title')}
  subtitle={m('settings.preferences.subtitle')}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Theme -->
  <FormSection title={m('settings.preferences.theme.title')} maxWidth="max-w-md">
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m('settings.preferences.theme.title')}
    >
      {#each themeOptions as option (option.value)}
        {@const isSelected = userPreferences.displayTheme === option.value}
        <ChoiceRow
          label={option.label}
          description={option.description}
          selected={isSelected}
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

  <!-- Language -->
  <FormSection title={m('settings.preferences.language.title')} maxWidth="max-w-md" bordered>
    <p class="mb-3 text-sm text-muted">{m('settings.preferences.language.description')}</p>

    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m('settings.preferences.language.title')}
    >
      {#each languageOptions as option (option.value)}
        {@const isSelected = activeLocale === option.value}
        <ChoiceRow
          label={option.label}
          selected={isSelected}
          onclick={() => handleLocaleSelect(option.value)}
        />
      {/each}
    </div>
  </FormSection>

  <!-- Timezone -->
  <FormSection title={m('settings.preferences.timezone.title')} maxWidth="max-w-md" bordered>
    <Combobox
      id="timezone"
      testid="timezone-input"
      label={m('settings.preferences.timezone.title')}
      labelHidden
      description={m('settings.preferences.timezone.description')}
      error={timezoneError}
      items={displayedTimezones}
      getValue={(timezone) => timezone}
      getLabel={(timezone) => timezone}
      placeholder={m('settings.preferences.timezone.browser_default')}
      clearLabel={m('settings.preferences.timezone.clear')}
      allowFreeform={false}
      bind:value={selectedTimezone}
      bind:text={timezoneSearch}
      ontextchange={handleTimezoneTextChange}
    />

    {#if selectedTimezoneTime}
      <p class="mt-1 text-sm text-muted">
        {m('settings.preferences.timezone.current_time', { time: selectedTimezoneTime })}
      </p>
    {/if}
  </FormSection>

  <!-- Time Format -->
  <FormSection title={m('settings.preferences.time_format.title')} maxWidth="max-w-md" bordered>
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m('settings.preferences.time_format.title')}
    >
      {#each timeFormatOptions as option (option.value)}
        {@const isSelected = selectedTimeFormat === option.value}
        <ChoiceRow
          label={option.label}
          description={option.description}
          selected={isSelected}
          onclick={() => (selectedTimeFormat = option.value)}
        />
      {/each}
    </div>
  </FormSection>

  <!-- Save -->
  <div class="flex max-w-md gap-2">
    <Button onclick={handleSave} disabled={!isModified || !!timezoneError}>
      {m('settings.preferences.save_button')}
    </Button>
  </div>
</div>
