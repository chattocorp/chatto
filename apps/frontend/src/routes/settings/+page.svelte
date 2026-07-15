<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { localeDisplayName, selectableLocales } from '$lib/i18n/locales';
  import { getLocale, type Locale } from '$lib/i18n/runtime';
  import { TimeFormat } from '$lib/render/types';
  import { clientSync } from '$lib/state/clientSync.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { userPreferences, type DisplayTheme } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, FormSection, Hint, PaneHeader } from '$lib/ui';
  import { Button, Combobox, FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatMessageTime } from '$lib/utils/formatTime';
  import { hour12ForTimeFormat } from '$lib/state/userSettings.svelte';
  import NotificationSoundSettings from '$lib/components/settings/NotificationSoundSettings.svelte';
  import * as m from '$lib/i18n/messages';

  const activeLocale = $derived(getLocale());
  const homeServer = $derived(serverRegistry.homeServer);
  const eligibleHomeServers = $derived(serverRegistry.clientSyncCandidates);
  const hasAuthenticatedServers = $derived(
    serverRegistry.servers.some((server) => serverRegistry.isAuthenticated(server.id))
  );
  const homeSupportsSync = $derived(
    homeServer ? serverRegistry.isClientSyncCapable(homeServer.id) : false
  );
  const syncLoading = $derived(clientSync.status === 'loading');
  const returnTo = $derived.by(() => {
    const candidate = page.url.searchParams.get('returnTo');
    return candidate?.startsWith('/') && !candidate.startsWith('//') ? candidate : resolve('/chat');
  });

  const allTimezones = Intl.supportedValuesOf('timeZone');
  let timezoneSearch = $state(clientSync.timezone ?? '');
  let selectedTimezone = $state(clientSync.timezone ?? '');
  let selectedTimeFormat = $state(clientSync.timeFormat);
  let adoptedPersonalSettingsKey = $state('');
  let isSaving = $state(false);
  let error = $state('');

  // Adopt portable values when the asynchronous home-server load completes,
  // then leave in-progress form edits alone until the source values change.
  $effect(() => {
    if (!clientSync.isInitialized) return;
    const key = JSON.stringify([
      clientSync.loadedHomeServerId,
      clientSync.timezone,
      clientSync.timeFormat
    ]);
    if (key === adoptedPersonalSettingsKey) return;
    adoptedPersonalSettingsKey = key;
    timezoneSearch = clientSync.timezone ?? '';
    selectedTimezone = clientSync.timezone ?? '';
    selectedTimeFormat = clientSync.timeFormat;
  });

  const displayedTimezones = $derived(
    (timezoneSearch
      ? allTimezones.filter((timezone) =>
          timezone.toLowerCase().includes(timezoneSearch.toLowerCase())
        )
      : allTimezones
    ).slice(0, 50)
  );
  const timezoneError = $derived(
    timezoneSearch && !allTimezones.includes(timezoneSearch)
      ? m['settings.preferences.timezone.invalid']()
      : undefined
  );
  const selectedTimezoneTime = $derived(
    selectedTimezone
      ? formatMessageTime(
          new Date(),
          {
            effectiveTimezone: selectedTimezone,
            effectiveHour12: hour12ForTimeFormat(selectedTimeFormat)
          },
          activeLocale
        )
      : null
  );
  const displayModified = $derived(
    (selectedTimezone || null) !== clientSync.timezone ||
      selectedTimeFormat !== clientSync.timeFormat
  );

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
  ] satisfies Array<{ value: DisplayTheme; label: string; description: string }>);

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
  ]);

  async function chooseHomeServer(id: string) {
    await clientSync.selectHomeServer(id);
  }

  async function chooseLocale(locale: Locale) {
    if (locale === activeLocale) return;
    try {
      await clientSync.setLocale(locale);
    } catch {
      toast.error(m['client_sync.settings.save_failed']());
    }
  }

  function handleTimezoneTextChange(text: string) {
    if (!text || allTimezones.includes(text)) selectedTimezone = text;
  }

  async function saveDisplaySettings() {
    if (timezoneError) return;
    isSaving = true;
    error = '';
    try {
      await clientSync.setDisplaySettings(selectedTimezone || null, selectedTimeFormat);
      toast.success(m['settings.preferences.saved']());
    } catch (caught) {
      error = caught instanceof Error ? caught.message : m['client_sync.settings.save_failed']();
    } finally {
      isSaving = false;
    }
  }

  function syncStatusLabel() {
    switch (clientSync.status) {
      case 'loading':
        return m['client_sync.settings.sync.loading']();
      case 'synced':
        return m['client_sync.settings.sync.synced']();
      case 'unavailable':
        return m['client_sync.settings.sync.unavailable']();
      case 'error':
        return m['client_sync.settings.sync.error']();
      default:
        return m['client_sync.settings.sync.local']();
    }
  }
</script>

<div class="flex min-h-0 flex-1 flex-col">
  <PaneHeader
    title={m['client_sync.settings.title']()}
    subtitle={m['client_sync.settings.subtitle']()}
    backHref={returnTo}
  />

  <div class="flex flex-col gap-7 overflow-y-auto p-6 md:p-8">
    <FormSection title={m['client_sync.settings.home.title']()} maxWidth="max-w-xl">
      <p class="mb-3 text-sm text-muted">{m['client_sync.settings.home.description']()}</p>

      {#if homeServer}
        <div class="flex items-center justify-between gap-4 surface-box px-4 py-3">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="iconify text-action uil--home" aria-hidden="true"></span>
              <strong class="truncate">{homeServer.name}</strong>
            </div>
            <p class="mt-1 truncate text-sm text-muted">{homeServer.url}</p>
          </div>
          <span class="rounded-full bg-surface-emphasized px-2 py-1 text-xs font-medium text-muted">
            {syncStatusLabel()}
          </span>
        </div>
        {#if clientSync.hasPendingRemoteHome}
          <div class="mt-3">
            <Hint tone="info">{m['client_sync.settings.home.choose_prompt']()}</Hint>
          </div>
        {/if}
      {:else if !hasAuthenticatedServers}
        <Hint tone="info">{m['client_sync.settings.home.add_server_first']()}</Hint>
      {:else if eligibleHomeServers.length === 0}
        <Hint tone="info">{m['client_sync.settings.home.no_capable_server']()}</Hint>
      {:else}
        <Hint tone="info">{m['client_sync.settings.home.choose_prompt']()}</Hint>
      {/if}

      {#if eligibleHomeServers.length > 1 ||
      (!homeServer && eligibleHomeServers.length > 0) ||
      (homeServer && !homeSupportsSync && eligibleHomeServers.length > 0) ||
      clientSync.hasPendingRemoteHome}
        <div class="mt-3 flex flex-col gap-2">
          {#each eligibleHomeServers as server (server.id)}
            <ChoiceRow
              label={server.name}
              description={server.url}
              selected={server.id === serverRegistry.homeServerId}
              onclick={() => chooseHomeServer(server.id)}
            />
          {/each}
        </div>
      {/if}
    </FormSection>

    <FormSection title={m['settings.preferences.theme.title']()} maxWidth="max-w-md" bordered>
      <div
        class="flex flex-col gap-2"
        role="radiogroup"
        aria-label={m['settings.preferences.theme.title']()}
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

    <FormSection title={m['settings.preferences.language.title']()} maxWidth="max-w-md" bordered>
      <p class="mb-3 text-sm text-muted">{m['settings.preferences.language.description']()}</p>
      <div
        class="flex flex-col gap-2"
        role="radiogroup"
        aria-label={m['settings.preferences.language.title']()}
      >
        {#each selectableLocales as locale (locale)}
          <ChoiceRow
            label={localeDisplayName(locale, activeLocale)}
            selected={activeLocale === locale}
            disabled={syncLoading}
            onclick={() => chooseLocale(locale)}
          />
        {/each}
      </div>
    </FormSection>

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
        disabled={syncLoading}
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

    <FormSection title={m['settings.preferences.time_format.title']()} maxWidth="max-w-md" bordered>
      <div
        class="flex flex-col gap-2"
        role="radiogroup"
        aria-label={m['settings.preferences.time_format.title']()}
      >
        {#each timeFormatOptions as option (option.value)}
          <ChoiceRow
            label={option.label}
            description={option.description}
            selected={selectedTimeFormat === option.value}
            disabled={syncLoading}
            onclick={() => (selectedTimeFormat = option.value)}
          />
        {/each}
      </div>
    </FormSection>

    {#if error}<div class="max-w-md"><FormError {error} /></div>{/if}
    <div class="max-w-md">
      <Button
        onclick={saveDisplaySettings}
        disabled={!displayModified || syncLoading || isSaving || !!timezoneError}
        loading={isSaving}
      >
        {m['settings.preferences.save_button']()}
      </Button>
    </div>

    <NotificationSoundSettings />
  </div>
</div>
