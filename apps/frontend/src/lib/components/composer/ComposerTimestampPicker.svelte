<script lang="ts">
  import { tick } from 'svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import * as m from '$lib/i18n/messages';
  import {
    createMessageTimestampToken,
    dateToDatetimeLocalValue,
    localDatetimeToEpochSeconds
  } from '$lib/messageTimestamps';
  import type { TipTapEditorApi } from './TipTapEditor.svelte';

  let {
    disabled,
    editorApi,
    effectiveTimezone
  }: {
    disabled: boolean;
    editorApi: TipTapEditorApi | null;
    effectiveTimezone?: string;
  } = $props();

  const timezoneListId = `timestamp-timezones-${Math.random().toString(36).slice(2)}`;
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
    if (!localValue) return m['composer.timestamp.error_required']();
    if (!timezoneValid) return m['composer.timestamp.error_timezone']();
    if (epochSeconds === null) return m['composer.timestamp.error_invalid']();
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

<button
  type="button"
  onpointerdown={(event) => event.preventDefault()}
  onclick={openPicker}
  bind:this={triggerElement}
  {disabled}
  class="flex h-6 w-6 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
  aria-label={m['composer.timestamp.insert_label']()}
  title={m['composer.timestamp.insert_label']()}
>
  <span class="iconify text-[15px] uil--clock"></span>
</button>

{#if pickerOpen}
  <ContextMenu
    anchor={pickerAnchor}
    role="dialog"
    ariaLabel={m['composer.timestamp.title']()}
    class="w-[min(22rem,calc(100vw-1rem))]"
    onclose={closePicker}
  >
    <form class="flex flex-col gap-1" onsubmit={insertTimestamp}>
      <header class="flex items-center gap-2 menu-section px-3 py-2 text-sm font-medium">
        <span class="iconify text-muted uil--clock"></span>
        <span>{m['composer.timestamp.title']()}</span>
      </header>

      <section class="flex flex-col gap-3 menu-section px-3 py-2">
        <label class="flex flex-col gap-1 text-sm">
          <span class="text-muted">{m['composer.timestamp.date_time']()}</span>
          <input
            class="input"
            type="datetime-local"
            name="timestamp-date-time"
            bind:this={dateTimeInput}
            bind:value={localValue}
            required
          />
        </label>

        <label class="flex flex-col gap-1 text-sm">
          <span class="text-muted">{m['composer.timestamp.timezone']()}</span>
          <input
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
        </label>

        {#if pickerError}
          <p class="form-error text-xs">{pickerError}</p>
        {/if}
      </section>

      <footer class="flex justify-end gap-2 menu-section px-3 py-2">
        <button type="button" class="btn-secondary btn-sm" onclick={() => closePicker()}>
          {m['common.cancel']()}
        </button>
        <button type="submit" class="btn-action btn-sm" disabled={pickerError !== null}>
          {m['composer.timestamp.insert']()}
        </button>
      </footer>
    </form>
  </ContextMenu>
{/if}
