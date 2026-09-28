<script lang="ts">
  import { CompactActionButton, ContextMenu } from '$lib/ui';
  import { tick } from 'svelte';
  import { Button, FormField, TextInput } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import {
    createMessageTimestampToken,
    dateToDatetimeLocalValue,
    localDatetimeToEpochSeconds
  } from '$lib/messageTimestamps';
  import type { ComposerEditorApi } from './editorTypes';

  let {
    disabled,
    editorApi,
    effectiveTimezone
  }: {
    disabled: boolean;
    editorApi: ComposerEditorApi | null;
    effectiveTimezone?: string;
  } = $props();

  const timezoneListId = `timestamp-timezones-${Math.random().toString(36).slice(2)}`;
  const dateTimeInputId = $props.id();
  const timezoneInputId = `${dateTimeInputId}-timezone`;
  const timezoneOptions = Intl.supportedValuesOf?.('timeZone') ?? [];
  let triggerElement = $state<HTMLButtonElement>();
  let dateTimeInput = $state<HTMLInputElement>();
  let pickerOpen = $state(false);
  let pickerAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let localValue = $state('');
  let timezoneSearch = $state('');
  const timezoneSuggestions = $derived(
    timezoneOptions
      .filter((timezone) => timezone.toLowerCase().includes(timezoneSearch.trim().toLowerCase()))
      .slice(0, 60)
  );
  const timezoneValid = $derived(isValidTimeZone(timezoneSearch));
  const epochSeconds = $derived(
    timezoneValid ? localDatetimeToEpochSeconds(localValue, timezoneSearch.trim()) : null
  );
  const pickerError = $derived.by(() => {
    if (!localValue) return m('composer.timestamp.error_required');
    if (!timezoneValid) return m('composer.timestamp.error_timezone');
    if (epochSeconds === null) return m('composer.timestamp.error_invalid');
    return null;
  });

  function browserTimeZone(): string {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  }

  function isValidTimeZone(timezone: string): boolean {
    const trimmed = timezone.trim();
    if (!trimmed) return false;
    try {
      new Intl.DateTimeFormat('en-US', { timeZone: trimmed }).format(new Date());
      return true;
    } catch {
      return false;
    }
  }

  function preferredTimeZone(): string {
    const timezone = effectiveTimezone ?? browserTimeZone();
    return isValidTimeZone(timezone) ? timezone : 'UTC';
  }

  function openPicker(event: MouseEvent): void {
    if (disabled) return;
    const button = event.currentTarget as HTMLButtonElement;
    const rect = button.getBoundingClientRect();
    const timezone = preferredTimeZone();
    triggerElement = button;
    timezoneSearch = timezone;
    localValue = dateToDatetimeLocalValue(new Date(Date.now() + 60 * 60_000), timezone);
    pickerAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    pickerOpen = true;
    tick().then(() => {
      if (!pickerOpen) return;
      dateTimeInput?.focus();
      dateTimeInput?.select();
    });
  }

  function closePicker({ restoreFocus = true }: { restoreFocus?: boolean } = {}): void {
    pickerOpen = false;
    pickerAnchor = null;
    if (restoreFocus) triggerElement?.focus();
  }

  function insertTimestamp(event: SubmitEvent): void {
    event.preventDefault();
    const timestamp = epochSeconds;
    if (timestamp === null || !editorApi) return;

    const token = createMessageTimestampToken(timestamp);
    const beforeCursor = editorApi.getTextBeforeCursor();
    const prefix = beforeCursor.length > 0 && !/\s$/.test(beforeCursor) ? ' ' : '';
    editorApi.insertText(`${prefix}${token} `);
    closePicker({ restoreFocus: false });
  }
</script>

<CompactActionButton
  wrapperClass="mobile-presentation:pill-button-group-touch"
  label={m('composer.timestamp.insert_label')}
  type="button"
  onpointerdown={(event) => event.preventDefault()}
  onclick={openPicker}
  {disabled}
  title={m('composer.timestamp.insert_label')}
>
  <span aria-hidden="true" class="iconify icon-[uil--clock] text-[15px]"></span>
</CompactActionButton>

{#if pickerOpen}
  <ContextMenu
    anchor={pickerAnchor}
    role="dialog"
    ariaLabel={m('composer.timestamp.title')}
    class="w-[min(22rem,calc(100vw-1rem))]"
    onclose={closePicker}
  >
    <form class="flex flex-col gap-1" onsubmit={insertTimestamp}>
      <header class="flex items-center gap-2 menu-section px-3 py-2 text-sm font-medium">
        <span aria-hidden="true" class="iconify icon-[uil--clock] text-muted"></span>
        <span>{m('composer.timestamp.title')}</span>
      </header>

      <section class="flex flex-col gap-3 menu-section px-3 py-2">
        <TextInput
          id={dateTimeInputId}
          type="datetime-local"
          name="timestamp-date-time"
          label={m('composer.timestamp.date_time')}
          bind:element={dateTimeInput}
          bind:value={localValue}
          required
        />

        <!-- A native datalist keeps the time zone suggestions inside this popover.
             Combobox would open a second floating layer above the context menu. -->
        <FormField id={timezoneInputId} label={m('composer.timestamp.timezone')} required>
          <input
            id={timezoneInputId}
            class="input"
            name="timestamp-timezone"
            list={timezoneListId}
            bind:value={timezoneSearch}
            autocomplete="off"
            spellcheck="false"
            required
          />
          <datalist id={timezoneListId}>
            {#each timezoneSuggestions as timezone (timezone)}
              <option value={timezone}></option>
            {/each}
          </datalist>
        </FormField>

        {#if pickerError}
          <p class="form-error">{pickerError}</p>
        {/if}
      </section>

      <footer class="flex justify-end gap-2 menu-section px-3 py-2">
        <Button size="sm" variant="secondary" onclick={() => closePicker()}>
          {m('common.cancel')}
        </Button>
        <Button type="submit" size="sm" disabled={pickerError !== null}>
          {m('composer.timestamp.insert')}
        </Button>
      </footer>
    </form>
  </ContextMenu>
{/if}
