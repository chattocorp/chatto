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
    icon,
    min,
    max,
    step = 1,
    disabled = false,
    rainbow = false,
    testid,
    oninput,
    onchange
  }: {
    id: string;
    label: string;
    value?: number;
    displayValue: string;
    icon?: string;
    min: number;
    max: number;
    step?: number;
    disabled?: boolean;
    /** Fade in a real rainbow track over the last 20%; animate the maximum readout. */
    rainbow?: boolean;
    testid?: string;
    oninput?: (event: Event) => void;
    onchange?: (event: Event) => void;
  } = $props();
  const progress = $derived(
    max > min ? Math.max(0, Math.min(1, ((value ?? min) - min) / (max - min))) : 0
  );
  const rainbowOpacity = $derived(rainbow && !disabled ? Math.max(0, (progress - 0.8) / 0.2) : 0);
  const celebrate = $derived(rainbow && !disabled && value === max);
</script>

<label for={id} class="flex flex-col gap-2 rounded-md bg-surface px-3 py-2.5">
  <span class="flex items-center justify-between gap-3 text-sm">
    <span class="flex min-w-0 items-center gap-2 font-medium text-text">
      {#if icon}
        <span class={['iconify shrink-0 text-base text-muted', icon]} aria-hidden="true"></span>
      {/if}
      <span>{label}</span>
    </span>
    <span class={['shrink-0 tabular-nums', celebrate ? 'awesome-text' : 'text-muted']}
      >{displayValue}</span
    >
  </span>
  <span class="relative block h-5">
    {#if rainbow && !disabled}
      <span
        aria-hidden="true"
        class="pointer-events-none absolute inset-x-0 top-1/2 h-2.5 -translate-y-1/2 overflow-hidden rounded-full bg-input-border"
      >
        <span
          class="absolute inset-y-0 start-0 overflow-hidden rounded-full bg-action"
          style:width={`${progress * 100}%`}
        >
          {#if rainbowOpacity > 0}
            <span
              class="absolute inset-0 overflow-hidden"
              style:opacity={rainbowOpacity}
              data-rainbow-band
            >
              <span class="rainbow-spectrum"></span>
            </span>
          {/if}
        </span>
      </span>
    {/if}
    <input
      {id}
      data-testid={testid}
      type="range"
      {min}
      {max}
      {step}
      bind:value
      {disabled}
      aria-valuetext={displayValue}
      {oninput}
      {onchange}
      class={[
        'relative h-5 w-full cursor-pointer accent-action disabled:cursor-not-allowed disabled:opacity-60',
        rainbow && !disabled && 'range-celebration'
      ]}
    />
  </span>
</label>
