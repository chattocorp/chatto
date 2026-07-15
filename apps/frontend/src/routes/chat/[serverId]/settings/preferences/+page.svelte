<script lang="ts">
  import * as m from '$lib/i18n/messages';
  import { localeDisplayName, selectableLocales } from '$lib/i18n/locales';
  import { getLocale, setLocale, type Locale } from '$lib/i18n/runtime';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createAccountAPI } from '$lib/api-client/account';
  import { TimeFormat } from '$lib/render/types';
  import { getUserSettings, hour12ForTimeFormat } from '$lib/state/userSettings.svelte';
  import { userPreferences, type DisplayTheme } from '$lib/state/userPreferences.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { ChoiceRow, PaneHeader, FormSection } from '$lib/ui';
  import { Button, Combobox, FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatMessageTime } from '$lib/utils/formatTime';

  const userSettings = getUserSettings();
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const connection = useConnection();
  const activeLocale = $derived(getLocale());

  function accountAPI() {
    const conn = connection();
    return createAccountAPI({
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  }

  // All available IANA timezone names
  const allTimezones = Intl.supportedValuesOf('timeZone');

  // Form state - initialize from current settings
  let timezoneSearch = $state(userSettings.timezone ?? '');
  let selectedTimezone = $state(userSettings.timezone ?? '');
  let selectedTimeFormat = $state<TimeFormat>(userSettings.timeFormat);
  let isSaving = $state(false);
  let error = $state('');

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
    (selectedTimezone || null) !== userSettings.timezone ||
      selectedTimeFormat !== userSettings.timeFormat
  );

  // Timezone validation
  const timezoneError = $derived.by(() => {
    if (!timezoneSearch) return undefined;
    if (allTimezones.includes(timezoneSearch)) return undefined;
    return m['settings.preferences.timezone.invalid']();
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

  async function handleSave() {
    // Validate timezone if set
    if (timezoneSearch && !allTimezones.includes(timezoneSearch)) {
      error = m['settings.preferences.timezone.invalid']();
      return;
    }

    isSaving = true;
    error = '';

    try {
      // Update the local settings state so formatting changes take effect immediately
      const settings = await accountAPI().updateSettings({
        timezone: selectedTimezone || null,
        timeFormat: selectedTimeFormat
      });
      userSettings.updateFromData(settings);
      if (currentUser.user) {
        currentUser.user = {
          ...currentUser.user,
          settings
        };
      }

      toast.success(m['settings.preferences.saved']());
    } catch (err) {
      error = err instanceof Error ? err.message : m['settings.preferences.save_failed']();
    } finally {
      isSaving = false;
    }
  }

  const themeOptions = $derived([
    {
      value: 'system',
      label: m['settings.preferences.theme.system.label'](),
      description: m['settings.preferences.theme.system.description']()
    },
    {
      value: 'light',
      label: m['settings.preferences.theme.light.label'](),
      description: m['settings.preferences.theme.light.description']()
    },
    {
      value: 'dark',
      label: m['settings.preferences.theme.dark.label'](),
      description: m['settings.preferences.theme.dark.description']()
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

  const timeFormatOptions = $derived([
    {
      value: TimeFormat.Auto,
      label: m['settings.preferences.time_format.browser_default.label'](),
      description: m['settings.preferences.time_format.browser_default.description']()
    },
    {
      value: TimeFormat.TwelveHour,
      label: m['settings.preferences.time_format.12h.label'](),
      description: m['settings.preferences.time_format.12h.description']()
    },
    {
      value: TimeFormat.TwentyFourHour,
      label: m['settings.preferences.time_format.24h.label'](),
      description: m['settings.preferences.time_format.24h.description']()
    }
  ] satisfies Array<{
    value: TimeFormat;
    label: string;
    description: string;
  }>);
</script>

<PaneHeader
  title={m['settings.preferences.title']()}
  subtitle={m['settings.preferences.subtitle']()}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Theme -->
  <FormSection title={m['settings.preferences.theme.title']()} maxWidth="max-w-md">
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m['settings.preferences.theme.title']()}
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

  <!-- Language -->
  <FormSection title={m['settings.preferences.language.title']()} maxWidth="max-w-md" bordered>
    <p class="mb-3 text-sm text-muted">{m['settings.preferences.language.description']()}</p>

    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m['settings.preferences.language.title']()}
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
  <FormSection title={m['settings.preferences.timezone.title']()} maxWidth="max-w-md" bordered>
    <Combobox
      id="timezone"
      testid="timezone-input"
      label={m['settings.preferences.timezone.title']()}
      labelHidden
      description={m['settings.preferences.timezone.description']()}
      error={timezoneError}
      items={displayedTimezones}
      getValue={(timezone) => timezone}
      getLabel={(timezone) => timezone}
      placeholder={m['settings.preferences.timezone.browser_default']()}
      clearLabel={m['settings.preferences.timezone.clear']()}
      allowFreeform={false}
      bind:value={selectedTimezone}
      bind:text={timezoneSearch}
      ontextchange={handleTimezoneTextChange}
    />

    {#if selectedTimezoneTime}
      <p class="mt-1 text-sm text-muted">
        {m['settings.preferences.timezone.current_time']({ time: selectedTimezoneTime })}
      </p>
    {/if}
  </FormSection>

  <!-- Time Format -->
  <FormSection title={m['settings.preferences.time_format.title']()} maxWidth="max-w-md" bordered>
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m['settings.preferences.time_format.title']()}
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
  {#if error}
    <div class="max-w-md">
      <FormError {error} />
    </div>
  {/if}

  <div class="flex max-w-md gap-2">
    <Button
      onclick={handleSave}
      disabled={!isModified || isSaving || !!timezoneError}
      loading={isSaving}
    >
      {m['settings.preferences.save_button']()}
    </Button>
  </div>
</div>
