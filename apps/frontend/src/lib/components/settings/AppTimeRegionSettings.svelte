<script lang="ts">
  import { Panel } from '$lib/components/admin';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { userPreferences, type DisplayClockFormat } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import { Combobox } from '$lib/ui/form';
  import { formatMessageTime, timeDisplaySettings } from '$lib/utils/formatTime';

  const activeLocale = $derived(getLocale());

  // All available IANA timezone names
  const allTimezones = Intl.supportedValuesOf('timeZone');

  let timezoneSearch = $state(userPreferences.timeZone);
  let selectedTimezone = $state(userPreferences.timeZone);

  // Filter timezone list based on search input
  let filteredTimezones = $derived(
    timezoneSearch
      ? allTimezones.filter((tz) => tz.toLowerCase().includes(timezoneSearch.toLowerCase()))
      : allTimezones
  );

  // Cap displayed results to avoid rendering 400+ items
  let displayedTimezones = $derived(filteredTimezones.slice(0, 50));

  function findTimezone(value: string): string | undefined {
    return allTimezones.find((tz) => tz.toLowerCase() === value.toLowerCase());
  }

  const timezoneError = $derived.by(() => {
    if (!timezoneSearch) return undefined;
    if (findTimezone(timezoneSearch)) return undefined;
    return m('settings.preferences.timezone.invalid');
  });

  function selectTimeZone(value: string) {
    selectedTimezone = value;
    userPreferences.timeZone = value;
  }

  function handleTimezoneTextChange(text: string) {
    if (!text) {
      selectTimeZone('');
      return;
    }
    // Typing an exact name applies it without requiring an explicit selection.
    const match = findTimezone(text);
    if (match) selectTimeZone(match);
  }

  const clockFormatOptions = $derived([
    {
      value: 'device',
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
    value: DisplayClockFormat;
    label: string;
    description: string;
  }>);

  const selectedTimezonePreview = $derived.by(() => {
    const settings = timeDisplaySettings();
    if (!userPreferences.timeZone) return null;
    return formatMessageTime(new Date(), settings, activeLocale);
  });
</script>

<!-- @component
  App-wide "Time & Region" display preferences. Lets the user pick an IANA
  timezone and a clock format that apply to every connected server in this
  browser; unset values follow the device defaults.
-->

<PageTitle title={m('settings.preferences.title')} />
<PaneHeader
  title={m('settings.preferences.title')}
  subtitle={m('settings.preferences.subtitle')}
/>

<PaneContent>
  <Panel title={m('settings.preferences.title')} icon="iconify icon-[uil--clock-three]">
    <div class="flex max-w-md flex-col gap-6">
      <p class="text-sm text-muted">{m('settings.preferences.browser_scope')}</p>

      <div>
        <Combobox
          id="timezone"
          testid="timezone-input"
          label={m('settings.preferences.timezone.title')}
          description={m('settings.preferences.timezone.description')}
          error={timezoneError}
          items={displayedTimezones}
          getValue={(timezone) => timezone}
          getLabel={(timezone) => timezone}
          placeholder={m('settings.preferences.timezone.browser_default')}
          clearLabel={m('settings.preferences.timezone.clear')}
          allowFreeform={false}
          bind:value={selectedTimezone}
          onselect={selectTimeZone}
          bind:text={timezoneSearch}
          ontextchange={handleTimezoneTextChange}
        />

        {#if selectedTimezonePreview}
          <p class="mt-1 text-sm text-muted">
            {m('settings.preferences.timezone.current_time', { time: selectedTimezonePreview })}
          </p>
        {/if}
      </div>

      <div class="border-t border-default pt-6">
        <div
          class="flex flex-col gap-2"
          role="radiogroup"
          aria-label={m('settings.preferences.time_format.title')}
        >
          {#each clockFormatOptions as option (option.value)}
            <ChoiceRow
              label={option.label}
              description={option.description}
              selected={userPreferences.clockFormat === option.value}
              onclick={() => (userPreferences.clockFormat = option.value)}
            />
          {/each}
        </div>
      </div>
    </div>
  </Panel>
</PaneContent>
