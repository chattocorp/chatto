<!-- @component Read-only progress with a continuous fill and the recessed finish of range controls. -->
<script lang="ts">
  let {
    value,
    label
  }: {
    /** Completion from 0 to 100. Out-of-range values are clamped; non-finite values show zero. */
    value: number;
    /** Accessible name for the operation in progress. */
    label: string;
  } = $props();
  const percent = $derived(Number.isFinite(value) ? Math.max(0, Math.min(100, value)) : 0);
</script>

<div
  role="progressbar"
  aria-label={label}
  aria-valuemin="0"
  aria-valuemax="100"
  aria-valuenow={percent}
  class="flex min-w-0 items-center gap-3"
>
  <div class="h-7 min-w-0 flex-1 control-frame bg-input p-[3px]" aria-hidden="true">
    <div class="relative h-full overflow-hidden rounded-lg">
      <div
        class="absolute inset-y-0 start-0 rounded-lg bg-(--color-button-action) shell-lighting transition-[width] duration-300 ease-out motion-reduce:transition-none"
        style:width={`${percent}%`}
      ></div>
    </div>
  </div>
  <span class="w-12 shrink-0 text-end font-medium text-text tabular-nums" aria-hidden="true"
    >{Math.floor(percent)}%</span
  >
</div>
