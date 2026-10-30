<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createAccountAPI, type UpdateSettingsInput } from '@chatto/client/api/account';
  import { Panel, PageTitle, ChoiceRow, FormSection, Hint, PaneContent, PaneHeader } from '$lib/ui';
  import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
  import { Button, Checkbox, Combobox, FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatMessageTime, hour12ForTimeFormat } from '$lib/utils/formatTime';

  const serverScope = useServerScope();
  const currentUser = $derived(serverScope.store.currentUser);
  const savedSettings = $derived(currentUser.user?.settings);
  const activeLocale = $derived(getLocale());

  function accountAPI() {
    return serverScope.connection.getAPI(createAccountAPI);
  }

  // All available IANA timezone names
  const allTimezones = Intl.supportedValuesOf('timeZone');

  // Only explicit edits override saved values. Replace the object on each edit
  // so a save response can distinguish its draft from newer input.
  let draft = $state<{
    timezoneSearch?: string;
    timezone?: string;
    timeFormat?: TimeFormat;
    shareTimezone?: boolean;
  }>({});
  const timezoneSearch = $derived(draft.timezoneSearch ?? savedSettings?.timezone ?? '');
  const selectedTimezone = $derived(draft.timezone ?? savedSettings?.timezone ?? '');
  const selectedTimeFormat = $derived(
    draft.timeFormat ?? savedSettings?.timeFormat ?? TimeFormat.TIME_FORMAT_AUTO
  );
  const shareTimezone = $derived(draft.shareTimezone ?? savedSettings?.shareTimezone ?? false);
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

  const supportsTimezoneSharing = $derived(savedSettings?.shareTimezone !== undefined);
  const changes = $derived.by(() => {
    const input: UpdateSettingsInput = {};
    if ((selectedTimezone || null) !== (savedSettings?.timezone || null)) {
      input.timezone = selectedTimezone || null;
    }
    if (selectedTimeFormat !== (savedSettings?.timeFormat ?? TimeFormat.TIME_FORMAT_AUTO)) {
      input.timeFormat = selectedTimeFormat;
    }
    if (supportsTimezoneSharing && shareTimezone !== savedSettings?.shareTimezone) {
      input.shareTimezone = shareTimezone;
    }
    return input;
  });
  const isModified = $derived(!currentUser.loading && Object.keys(changes).length > 0);

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
    if (!text || allTimezones.includes(text)) draft = { ...draft, timezone: text };
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    if (!isModified || isSaving || timezoneError) return;

    isSaving = true;
    error = '';
    const userId = serverScope.store.accountId;
    const submittedDraft = draft;

    try {
      const settings = await accountAPI().updateSettings(changes);
      if (!serverScope.isCurrent()) return;
      currentUser.update(userId, () => ({ settings }));
      if (draft === submittedDraft) draft = {};

      toast.success(m('settings.preferences.saved'));
    } catch (err) {
      if (!serverScope.isCurrent()) return;
      error = errorMessage(err, m('settings.preferences.save_failed'));
    } finally {
      if (serverScope.isCurrent()) isSaving = false;
    }
  }

  const timeFormatOptions = $derived([
    {
      value: TimeFormat.TIME_FORMAT_AUTO,
      label: m('settings.preferences.time_format.browser_default.label'),
      description: m('settings.preferences.time_format.browser_default.description')
    },
    {
      value: TimeFormat.TIME_FORMAT_12_HOUR,
      label: m('settings.preferences.time_format.12h.label'),
      description: m('settings.preferences.time_format.12h.description')
    },
    {
      value: TimeFormat.TIME_FORMAT_24_HOUR,
      label: m('settings.preferences.time_format.24h.label'),
      description: m('settings.preferences.time_format.24h.description')
    }
  ] satisfies Array<{
    value: TimeFormat;
    label: string;
    description: string;
  }>);
</script>

<PageTitle title={m('settings.preferences.title')} />

<div class="pane-page">
  <PaneHeader
    title={m('settings.preferences.title')}
    subtitle={m('settings.preferences.subtitle')}
  />

  <PaneContent>
    <Panel title={m('settings.preferences.title')} icon="iconify icon-[uil--clock-three]">
      <form class="flex max-w-md flex-col gap-6" onsubmit={handleSubmit}>
        <FormSection title={m('settings.preferences.timezone.title')}>
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
            disabled={currentUser.loading}
            bind:value={() => selectedTimezone, (timezone) => (draft = { ...draft, timezone })}
            bind:text={
              () => timezoneSearch, (timezoneSearch) => (draft = { ...draft, timezoneSearch })
            }
            ontextchange={handleTimezoneTextChange}
          />

          {#if selectedTimezoneTime}
            <p class="mt-1 text-sm text-muted">
              {m('settings.preferences.timezone.current_time', {
                time: selectedTimezoneTime
              })}
            </p>
          {/if}
        </FormSection>

        <FormSection title={m('settings.preferences.timezone_sharing.title')} bordered>
          {#if currentUser.loading || supportsTimezoneSharing}
            <Checkbox
              id="share-timezone"
              label={m('settings.preferences.timezone_sharing.label')}
              description={m('settings.preferences.timezone_sharing.description')}
              disabled={currentUser.loading}
              bind:checked={
                () => shareTimezone, (shareTimezone) => (draft = { ...draft, shareTimezone })
              }
            />
          {:else}
            <Hint tone="warning">
              {m('settings.preferences.timezone_sharing.unsupported')}
            </Hint>
          {/if}
        </FormSection>

        <FormSection title={m('settings.preferences.time_format.title')} bordered>
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
                disabled={currentUser.loading}
                onclick={() => (draft = { ...draft, timeFormat: option.value })}
              />
            {/each}
          </div>
        </FormSection>

        {#if error}
          <FormError {error} />
        {/if}

        <div class="flex gap-2">
          <Button
            type="submit"
            disabled={!isModified || isSaving || !!timezoneError}
            loading={isSaving}
          >
            {m('settings.preferences.save_button')}
          </Button>
        </div>
      </form>
    </Panel>
  </PaneContent>
</div>
