<!--
@component

Standard labeled range control for settings. Owns the value readout, optional
icon, disabled state, and field spacing.

The visible control is drawn from design-system primitives: a recessed well
like text inputs, a lit fill in the primary action colour, and a raised grip
like a segmented selection. A transparent native range input sits on top of
the travel area, so keyboard, pointer, and assistive-technology behaviour stay
native. Its thumb has the grip's width, so pointer positions map exactly onto
the drawn grip.
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
    /** Visible stops on the track, independent of the keyboard/pointer step size. */
    ticks?: readonly number[];
    disabled?: boolean;
    testid?: string;
    oninput?: (event: Event) => void;
    onchange?: (event: Event) => void;
  } = $props();

  /** Position of a value along the travel, from 0 to 1. */
  function fraction(position: number): number {
    return max > min ? Math.max(0, Math.min(1, (position - min) / (max - min))) : 0;
  }

  const progress = $derived(fraction(value ?? (min + max) / 2));
</script>

<label
  for={id}
  class={[
    'flex flex-col gap-2.5 rounded-md bg-surface px-3 py-2.5 range-field',
    disabled && 'opacity-60'
  ]}
>
  <span class="flex items-center justify-between gap-3 text-sm">
    <span class="flex min-w-0 items-center gap-2 font-medium text-text">
      {#if icon}
        <span class={['iconify shrink-0 text-base text-muted', icon]} aria-hidden="true"></span>
      {/if}
      <span>{label}</span>
    </span>
    <span class="shrink-0 text-muted tabular-nums">{displayValue}</span>
  </span>
  <span class="range-track" style:--range-progress={progress}>
    <span class="range-travel" aria-hidden="true">
      <span class="range-fill"></span>
      {#each ticks ?? [] as tick (tick)}
        <span
          class={['range-tick', fraction(tick) <= progress && 'range-tick-filled']}
          style:--range-tick={fraction(tick)}
        ></span>
      {/each}
      <span class="range-grip"></span>
    </span>
    <input
      {id}
      data-testid={testid}
      type="range"
      {min}
      {max}
      {step}
      bind:value
      {disabled}
      aria-valuetext={ariaValueText ?? displayValue}
      aria-describedby={describedBy}
      {oninput}
      {onchange}
      class="range-input"
    />
  </span>
</label>
