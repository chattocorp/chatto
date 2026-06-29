<script lang="ts">
  import * as m from '$lib/i18n/messages';
  import { getLocale, setLocale, type Locale } from '$lib/i18n/runtime';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createAccountAPI } from '@chatto/api-client/account';
  import { TimeFormat } from '$lib/render/types';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { userPreferences, type DisplayTheme } from '$lib/state/userPreferences.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { PaneHeader, FormSection } from '$lib/ui';
  import { Button, FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

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
  let selectedTimezone = $state<string | null>(userSettings.timezone);
  let selectedTimeFormat = $state<TimeFormat>(userSettings.timeFormat);
  let isSaving = $state(false);
  let error = $state('');

  // Dropdown state
  let dropdownOpen = $state(false);
  let highlightedIndex = $state(-1);
  let listRef = $state<HTMLUListElement | null>(null);

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
    selectedTimezone !== userSettings.timezone || selectedTimeFormat !== userSettings.timeFormat
  );

  // Timezone validation
  const timezoneError = $derived.by(() => {
    if (!timezoneSearch) return undefined;
    if (allTimezones.includes(timezoneSearch)) return undefined;
    return m['settings.preferences.timezone.invalid']();
  });

  const selectedTimezoneTime = $derived.by(() => {
    if (!selectedTimezone) return null;

    return new Date().toLocaleTimeString(activeLocale, {
      timeZone: selectedTimezone,
      hour: '2-digit',
      minute: '2-digit',
      hour12: userSettings.effectiveHour12
    });
  });

  function handleTimezoneInput(e: Event) {
    const value = (e.target as HTMLInputElement).value;
    timezoneSearch = value;
    highlightedIndex = -1;

    if (value === '') {
      selectedTimezone = null;
      dropdownOpen = false;
    } else if (allTimezones.includes(value)) {
      selectedTimezone = value;
      dropdownOpen = false;
    } else {
      dropdownOpen = true;
    }
  }

  function selectTimezone(tz: string) {
    timezoneSearch = tz;
    selectedTimezone = tz;
    dropdownOpen = false;
  }

  function handleClearTimezone() {
    timezoneSearch = '';
    selectedTimezone = null;
    dropdownOpen = false;
  }

  function handleLocaleSelect(locale: Locale) {
    if (locale === activeLocale) return;
    void setLocale(locale);
  }

  function handleTimezoneKeydown(e: KeyboardEvent) {
    if (!dropdownOpen) {
      if (e.key === 'ArrowDown' && timezoneSearch) {
        dropdownOpen = true;
        highlightedIndex = 0;
        e.preventDefault();
      }
      return;
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        highlightedIndex = Math.min(highlightedIndex + 1, displayedTimezones.length - 1);
        listRef?.children[highlightedIndex]?.scrollIntoView({ block: 'nearest' });
        break;
      case 'ArrowUp':
        e.preventDefault();
        highlightedIndex = Math.max(highlightedIndex - 1, 0);
        listRef?.children[highlightedIndex]?.scrollIntoView({ block: 'nearest' });
        break;
      case 'Enter':
        e.preventDefault();
        if (highlightedIndex >= 0 && highlightedIndex < displayedTimezones.length) {
          selectTimezone(displayedTimezones[highlightedIndex]);
        }
        break;
      case 'Escape':
        dropdownOpen = false;
        break;
    }
  }

  function handleTimezoneBlur() {
    // Delay to allow click on dropdown item to register
    setTimeout(() => {
      dropdownOpen = false;
    }, 150);
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
        timezone: selectedTimezone,
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

  const languageOptions = $derived([
    {
      value: 'en',
      label: m['settings.preferences.language.english']()
    },
    {
      value: 'de',
      label: m['settings.preferences.language.german']()
    }
  ] satisfies Array<{
    value: Locale;
    label: string;
  }>);

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
        <button
          type="button"
          role="radio"
          aria-checked={isSelected}
          class={['choice-row', isSelected && 'choice-row-selected']}
          onclick={() => (userPreferences.displayTheme = option.value)}
        >
          <span class={['choice-indicator', isSelected && 'choice-indicator-selected']}>
            {#if isSelected}
              <span class="choice-indicator-dot"></span>
            {/if}
          </span>
          <div>
            <div class={isSelected ? 'font-medium' : ''}>{option.label}</div>
            <div class="text-sm text-muted">{option.description}</div>
          </div>
        </button>
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
        <button
          type="button"
          role="radio"
          aria-checked={isSelected}
          class={['choice-row', isSelected && 'choice-row-selected']}
          onclick={() => handleLocaleSelect(option.value)}
        >
          <span class={['choice-indicator', isSelected && 'choice-indicator-selected']}>
            {#if isSelected}
              <span class="choice-indicator-dot"></span>
            {/if}
          </span>
          <div>
            <div class={isSelected ? 'font-medium' : ''}>{option.label}</div>
          </div>
        </button>
      {/each}
    </div>
  </FormSection>

  <!-- Timezone -->
  <FormSection title={m['settings.preferences.timezone.title']()} maxWidth="max-w-md" bordered>
    <p class="mb-3 text-sm text-muted">{m['settings.preferences.timezone.description']()}</p>

    <div class="relative">
      <input
        type="text"
        data-testid="timezone-input"
        value={timezoneSearch}
        oninput={handleTimezoneInput}
        onfocus={() => {
          if (timezoneSearch && !allTimezones.includes(timezoneSearch)) dropdownOpen = true;
        }}
        onblur={handleTimezoneBlur}
        onkeydown={handleTimezoneKeydown}
        placeholder={m['settings.preferences.timezone.browser_default']()}
        class="input w-full"
        autocomplete="off"
        role="combobox"
        aria-expanded={dropdownOpen}
        aria-controls="timezone-listbox"
        aria-autocomplete="list"
      />
      {#if timezoneSearch}
        <button
          type="button"
          class="absolute top-1/2 right-2 icon-action -translate-y-1/2"
          onclick={handleClearTimezone}
          title={m['settings.preferences.timezone.clear']()}
        >
          <span class="iconify uil--times"></span>
        </button>
      {/if}

      {#if dropdownOpen && displayedTimezones.length > 0}
        <ul
          id="timezone-listbox"
          role="listbox"
          bind:this={listRef}
          class="absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded-lg border border-border bg-surface shadow-lg"
        >
          {#each displayedTimezones as tz, i (tz)}
            <li
              role="option"
              aria-selected={i === highlightedIndex}
              class={[
                'cursor-pointer px-3 py-1.5 text-sm',
                i === highlightedIndex
                  ? 'bg-accent/20 text-text'
                  : 'text-muted hover:bg-surface-100 hover:text-text'
              ]}
              onmousedown={() => selectTimezone(tz)}
            >
              {tz}
            </li>
          {/each}
          {#if filteredTimezones.length > 50}
            <li class="px-3 py-1.5 text-xs text-muted">
              {m['settings.preferences.timezone.more_results']({ count: filteredTimezones.length - 50 })}
            </li>
          {/if}
        </ul>
      {/if}
    </div>

    {#if timezoneError}
      <p class="mt-1 text-sm text-danger">{timezoneError}</p>
    {/if}

    {#if selectedTimezoneTime}
      <p class="mt-1 text-sm text-muted">
        {m['settings.preferences.timezone.current_time']({ time: selectedTimezoneTime })}
      </p>
    {/if}
  </FormSection>

  <!-- Time Format -->
  <FormSection title={m['settings.preferences.time_format.title']()} maxWidth="max-w-md" bordered>
    <div class="flex flex-col gap-2">
      {#each timeFormatOptions as option (option.value)}
        {@const isSelected = selectedTimeFormat === option.value}
        <button
          type="button"
          class={['choice-row', isSelected && 'choice-row-selected']}
          onclick={() => (selectedTimeFormat = option.value)}
        >
          <span class={['choice-indicator', isSelected && 'choice-indicator-selected']}>
            {#if isSelected}
              <span class="choice-indicator-dot"></span>
            {/if}
          </span>
          <div>
            <div class={isSelected ? 'font-medium' : ''}>{option.label}</div>
            <div class="text-sm text-muted">{option.description}</div>
          </div>
        </button>
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
