<!--
@component

Standard labeled range control for settings. Owns the value readout, optional
icon, disabled state, semantic action color, and field spacing.
-->
<script lang="ts">
  let {
    id,
    label,
    value = $bindable(),
    displayValue,
    ariaValueText,
    describedBy,
    icon,
    min,
    max,
    step = 1,
    ticks,
    prominent = false,
    disabled = false,
    testid,
    oninput,
    onchange
  }: {
    id: string;
    label: string;
    value?: number;
    displayValue: string;
    /** Describes the effect of the current value to assistive technology. */
    ariaValueText?: string;
    /** ID of visible helper text for the input. */
    describedBy?: string;
    icon?: string;
    min: number;
    max: number;
    step?: number;
    /** Native reference marks, independent of the keyboard/pointer step size. */
    ticks?: readonly number[];
    /** Gives a primary setting a larger track, thumb, and pointer target. */
    prominent?: boolean;
    disabled?: boolean;
    testid?: string;
    oninput?: (event: Event) => void;
    onchange?: (event: Event) => void;
  } = $props();

  const progress = $derived(
    max > min
      ? `${Math.max(0, Math.min(100, (((value ?? (min + max) / 2) - min) / (max - min)) * 100))}%`
      : '0%'
  );
</script>

<label
  for={id}
  class={[
    'flex flex-col gap-2 rounded-md bg-surface px-3 py-2.5',
    prominent && 'range-prominent-field'
  ]}
>
  <span class="flex items-center justify-between gap-3 text-sm">
    <span class="flex min-w-0 items-center gap-2 font-medium text-text">
      {#if icon}
        <span class={['iconify shrink-0 text-base text-muted', icon]} aria-hidden="true"></span>
      {/if}
      <span>{label}</span>
    </span>
    <span class="shrink-0 tabular-nums">
      <span class="text-muted">{displayValue}</span>
    </span>
  </span>
  <span class={['relative block', prominent ? 'h-11' : 'h-5']}>
    <input
      {id}
      data-testid={testid}
      type="range"
      {min}
      {max}
      {step}
      list={ticks?.length ? `${id}-ticks` : undefined}
      bind:value
      {disabled}
      aria-valuetext={ariaValueText ?? displayValue}
      aria-describedby={describedBy}
      {oninput}
      {onchange}
      style:--range-progress={prominent ? progress : undefined}
      class={[
        'relative w-full cursor-pointer accent-action disabled:cursor-not-allowed disabled:opacity-60',
        prominent ? 'range-prominent h-11' : 'h-5'
      ]}
    />
    {#if ticks?.length}
      <datalist id={`${id}-ticks`}>
        {#each ticks as tick (tick)}
          <option value={tick}></option>
        {/each}
      </datalist>
    {/if}
  </span>
</label>
