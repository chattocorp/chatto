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
    testid?: string;
    oninput?: (event: Event) => void;
    onchange?: (event: Event) => void;
  } = $props();
</script>

<label for={id} class="flex flex-col gap-2 rounded-md bg-surface px-3 py-2.5">
  <span class="flex items-center justify-between gap-3 text-sm">
    <span class="flex min-w-0 items-center gap-2 font-medium text-text">
      {#if icon}
        <span class={['iconify shrink-0 text-base text-muted', icon]} aria-hidden="true"></span>
      {/if}
      <span>{label}</span>
    </span>
    <span class="shrink-0 text-muted tabular-nums">{displayValue}</span>
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
    aria-valuetext={displayValue}
    {oninput}
    {onchange}
    class="w-full cursor-pointer accent-action disabled:cursor-not-allowed disabled:opacity-60"
  />
</label>
