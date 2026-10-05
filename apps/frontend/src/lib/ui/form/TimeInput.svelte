<!--
  @component
  A time-of-day field that shows a 12-hour or 24-hour clock as the caller requests.

  Native time inputs take their clock from the browser or operating system locale,
  and pages cannot change it (whatwg/html#6698). Use this field when the viewer's
  time format preference must apply.

  The bound value is a 24-hour `HH:mm` string, like the value of a native time
  input. It is empty while the entry is incomplete or not valid.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { dayPeriodLabels } from '$lib/utils/formatTime';

  type Period = 'am' | 'pm';
  /** The text in each segment, which can be incomplete while the user types. */
  type Draft = { hour: string; minute: string; period: Period };

  let {
    id,
    value = $bindable(''),
    hour12,
    disabled = false
  }: {
    /** Set on the hour segment, so a label or focus target can point to the field. */
    id?: string;
    value?: string;
    /** Show hours 1–12 with a day period instead of hours 0–23. */
    hour12: boolean;
    disabled?: boolean;
  } = $props();

  const periods = $derived(dayPeriodLabels());
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
    const digits = input.value.replace(/\D/g, '').slice(0, 2);
    // Svelte does not write the DOM when the cleaned text equals the current state.
    input.value = digits;
    update({ [segment]: digits });
    if (segment === 'hour' && digits.length === 2) minuteInput?.focus();
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
    aria-label={m('ui.form.time.hour')}
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
    aria-label={m('ui.form.time.minute')}
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
