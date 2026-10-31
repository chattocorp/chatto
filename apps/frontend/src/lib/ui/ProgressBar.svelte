<!-- @component Read-only segmented progress with the recessed finish of range controls. -->
<script lang="ts">
  let { value, label }: { value: number; label: string } = $props();
  const percent = $derived(Number.isFinite(value) ? Math.max(0, Math.min(100, value)) : 0);
  const segments = Array.from({ length: 40 }, (_, index) => index);
</script>

<div
  role="progressbar"
  aria-label={label}
  aria-valuemin="0"
  aria-valuemax="100"
  aria-valuenow={percent}
  class="flex min-w-0 items-center gap-4"
>
  <div
    class="flex h-8 min-w-0 flex-1 gap-0.5 control-frame bg-input p-1 sm:gap-1"
    aria-hidden="true"
  >
    {#each segments as segment (segment)}
      <span class="relative min-w-0 flex-1 overflow-hidden rounded-xs bg-text/10">
        <span
          class="absolute inset-y-0 start-0 rounded-xs bg-(--color-button-action) shell-lighting transition-[width] duration-300 motion-reduce:transition-none"
          style:width={`${Math.max(0, Math.min(1, percent / 2.5 - segment)) * 100}%`}
        ></span>
      </span>
    {/each}
  </div>
  <span class="w-12 shrink-0 text-end font-semibold text-action tabular-nums" aria-hidden="true"
    >{Math.floor(percent)}%</span
  >
</div>
