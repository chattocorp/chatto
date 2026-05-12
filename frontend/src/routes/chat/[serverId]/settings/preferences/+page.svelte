<script lang="ts">
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { graphql } from '$lib/gql';
  import { TimeFormat } from '$lib/gql/graphql';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { PaneHeader, FormSection } from '$lib/ui';
  import { Button, FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const userSettings = getUserSettings();
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);

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
    return 'Please select a valid timezone from the list';
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
      error = 'Please select a valid timezone from the list';
      return;
    }

    isSaving = true;
    error = '';

    try {
      const result = await graphqlClientManager.originClient.client
        .mutation(
          graphql(`
            mutation UpdateSettings($input: UpdateSettingsInput!) {
              updateSettings(input: $input) {
                timezone
                timeFormat
              }
            }
          `),
          {
            input: {
              userId: currentUser.user!.id,
              // Send empty string to clear (Go backend: nil = no change, "" = clear)
              timezone: selectedTimezone ?? '',
              timeFormat: selectedTimeFormat
            }
          }
        )
        .toPromise();

      if (result.error) {
        error = result.error.message;
        return;
      }

      // Update the local settings state so formatting changes take effect immediately
      const data = result.data?.updateSettings;
      if (data) {
        userSettings.updateFromData(data);
      }

      toast.success('Preferences saved');
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save preferences';
    } finally {
      isSaving = false;
    }
  }

  const timeFormatOptions = [
    {
      value: TimeFormat.Unspecified,
      label: 'Browser default',
      description: 'Use your browser locale to determine 12h or 24h format'
    },
    {
      value: TimeFormat.TwelveHour,
      label: '12-hour',
      description: 'e.g., 2:30 PM'
    },
    {
      value: TimeFormat.TwentyFourHour,
      label: '24-hour',
      description: 'e.g., 14:30'
    }
  ];
</script>

<PaneHeader title="Preferences" subtitle="Customize your display settings" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Timezone -->
  <FormSection title="Timezone" maxWidth="max-w-md">
    <p class="mb-3 text-sm text-muted">
      Set your timezone to display timestamps in your local time. Leave empty to use your browser's
      default.
    </p>

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
        placeholder="Browser default"
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
          class="hover:text-foreground absolute top-1/2 right-2 -translate-y-1/2 cursor-pointer rounded p-1 text-muted"
          onclick={handleClearTimezone}
          title="Clear timezone (use browser default)"
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
                  ? 'text-foreground bg-accent/20'
                  : 'hover:text-foreground text-muted hover:bg-surface-100'
              ]}
              onmousedown={() => selectTimezone(tz)}
            >
              {tz}
            </li>
          {/each}
          {#if filteredTimezones.length > 50}
            <li class="px-3 py-1.5 text-xs text-muted">
              {filteredTimezones.length - 50} more — type to narrow results
            </li>
          {/if}
        </ul>
      {/if}
    </div>

    {#if timezoneError}
      <p class="mt-1 text-sm text-danger">{timezoneError}</p>
    {/if}

    {#if selectedTimezone}
      <p class="mt-1 text-sm text-muted">
        Current time there: {new Date().toLocaleTimeString('en-US', {
          timeZone: selectedTimezone,
          hour: '2-digit',
          minute: '2-digit',
          hour12: userSettings.effectiveHour12
        })}
      </p>
    {/if}
  </FormSection>

  <!-- Time Format -->
  <FormSection title="Time Format" maxWidth="max-w-md" bordered>
    <div class="flex flex-col gap-2">
      {#each timeFormatOptions as option (option.value)}
        {@const isSelected = selectedTimeFormat === option.value}
        <button
          type="button"
          class={[
            'flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2 text-left transition-colors',
            isSelected
              ? 'border-accent bg-accent/10'
              : 'hover:border-border-highlighted border-border hover:bg-surface-100'
          ]}
          onclick={() => (selectedTimeFormat = option.value)}
        >
          <span
            class={[
              'flex h-5 w-5 shrink-0 items-center justify-center rounded-full border-2 transition-colors',
              isSelected ? 'border-accent bg-accent' : 'border-muted'
            ]}
          >
            {#if isSelected}
              <span class="h-2 w-2 rounded-full bg-white"></span>
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
      Save Preferences
    </Button>
  </div>
</div>
