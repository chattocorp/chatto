<!--
  @component
  A time-of-day field that shows a 12-hour or 24-hour clock as the caller requests.

  Native time inputs take their clock from the browser or operating system locale,
  and pages cannot change it (whatwg/html#6698). Use this field when the viewer's
  time format preference must apply.

  The bound value is a 24-hour `HH:mm` string, like the value of a native time
  input. It is empty while the entry is incomplete or not valid. When the caller
  sets the value to empty, a partial entry that the user typed stays visible.

  Each segment has its own accessible name. Label the complete field with a
  `FormField` that has `group` set.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { dayPeriodLabels } from '$lib/utils/dayPeriods';

  type Period = 'am' | 'pm';
  /** The text in each segment, which can be incomplete while the user types. */
  type Draft = { hour: string; minute: string; period: Period };

  let {
    id,
    value = $bindable(''),
    hour12,
    disabled = false
  }: {
    /** Set on the hour segment, so callers can focus the field. */
    id?: string;
    value?: string;
    /** Show hours 1–12 with a day period instead of hours 0–23. */
    hour12: boolean;
    disabled?: boolean;
  } = $props();

  const periods = $derived(dayPeriodLabels(getLocale()));
  const hourRange = $derived(hour12 ? [1, 12] : [0, 23]);
  let minuteInput = $state<HTMLInputElement>();
  let draft = $state<Draft | null>(null);
  // Keep the segment text while it describes the bound value. This keeps partial
  // entries, such as "1" before "13", and replaces them when the caller sets a value.
  const shown = $derived(
    draft && valueFor(draft, hour12) === value ? draft : draftFor(value, hour12)
  );

  function pad(n: number): string {
    return String(n).padStart(2, '0');
  }

  function draftFor(value: string, hour12: boolean): Draft {
    const match = /^(\d{2}):(\d{2})$/.exec(value);
    if (!match) return { hour: '', minute: '', period: 'am' };
    const hour = Number(match[1]);
    return {
      hour: hour12 ? pad(hour % 12 || 12) : match[1],
      minute: match[2],
      period: hour >= 12 ? 'pm' : 'am'
    };
  }

  function valueFor(draft: Draft, hour12: boolean): string {
    if (!/^\d{1,2}$/.test(draft.hour) || !/^\d{1,2}$/.test(draft.minute)) return '';
    let hour = Number(draft.hour);
    const minute = Number(draft.minute);
    if (minute > 59) return '';
    if (hour12) {
      if (hour < 1 || hour > 12) return '';
      hour = (hour % 12) + (draft.period === 'pm' ? 12 : 0);
    } else if (hour > 23) {
      return '';
    }
    return `${pad(hour)}:${pad(minute)}`;
  }

  function update(patch: Partial<Draft>): void {
    draft = { ...shown, ...patch };
    value = valueFor(draft, hour12);
  }

  function inputDigits(
    event: Event & { currentTarget: HTMLInputElement },
    segment: 'hour' | 'minute'
  ) {
    const input = event.currentTarget;
    // Keep the newest digits, so typing into a filled segment replaces its value.
    const digits = input.value.replace(/\D/g, '').slice(-2);
    // Svelte does not write the DOM when the cleaned text equals the current state.
    input.value = digits;
    update({ [segment]: digits });
    if (segment === 'hour' && hourComplete(digits)) minuteInput?.focus();
  }

  /** True when the hour is valid and no further digit can follow it. */
  function hourComplete(digits: string): boolean {
    const hour = Number(digits);
    const [min, max] = hourRange;
    return digits !== '' && hour >= min && hour <= max && (digits.length === 2 || hour * 10 > max);
  }

  /** Step a segment with the arrow keys, wrapping like a native time input. */
  function stepDigits(event: KeyboardEvent, segment: 'hour' | 'minute') {
    const delta = event.key === 'ArrowUp' ? 1 : event.key === 'ArrowDown' ? -1 : 0;
    if (!delta) return;
    event.preventDefault();
    const current = Number(shown[segment] || 0);
    let next: number;
    if (segment === 'minute') next = (current + delta + 60) % 60;
    else if (hour12) next = ((current - 1 + delta + 12) % 12) + 1;
    else next = (current + delta + 24) % 24;
    update({ [segment]: pad(next) });
  }

  function padOnBlur(segment: 'hour' | 'minute') {
    const text = shown[segment];
    if (text.length === 1) update({ [segment]: pad(Number(text)) });
  }
</script>

<div
  class="flex input w-fit shrink-0 items-center gap-0.5 tabular-nums focus-within:border-action"
  class:opacity-60={disabled}
  dir="ltr"
>
  <input
    {id}
    class="w-[2ch] bg-transparent text-center outline-none"
    inputmode="numeric"
    autocomplete="off"
    placeholder="--"
    role="spinbutton"
    aria-label={m('ui.form.time.hour')}
    aria-valuemin={hourRange[0]}
    aria-valuemax={hourRange[1]}
    aria-valuenow={shown.hour === '' ? undefined : Number(shown.hour)}
    value={shown.hour}
    {disabled}
    oninput={(event) => inputDigits(event, 'hour')}
    onkeydown={(event) => stepDigits(event, 'hour')}
    onfocus={(event) => event.currentTarget.select()}
    onblur={() => padOnBlur('hour')}
  />
  <span aria-hidden="true" class="text-muted">:</span>
  <input
    bind:this={minuteInput}
    class="w-[2ch] bg-transparent text-center outline-none"
    inputmode="numeric"
    autocomplete="off"
    placeholder="--"
    role="spinbutton"
    aria-label={m('ui.form.time.minute')}
    aria-valuemin={0}
    aria-valuemax={59}
    aria-valuenow={shown.minute === '' ? undefined : Number(shown.minute)}
    value={shown.minute}
    {disabled}
    oninput={(event) => inputDigits(event, 'minute')}
    onkeydown={(event) => stepDigits(event, 'minute')}
    onfocus={(event) => event.currentTarget.select()}
    onblur={() => padOnBlur('minute')}
  />
  {#if hour12}
    <select
      class="ms-1 cursor-pointer bg-transparent outline-none"
      aria-label={m('ui.form.time.period')}
      value={shown.period}
      {disabled}
      onchange={(event) => update({ period: event.currentTarget.value as Period })}
    >
      <option value="am">{periods.am}</option>
      <option value="pm">{periods.pm}</option>
    </select>
  {/if}
</div>
