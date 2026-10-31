<script lang="ts">
  import type { TokenUsage } from 'runling';
  import { tokenCount } from '$lib/usage.ts';

  /** Run totals already include child activities; do not sum the activity tree again. */
  let { usage }: { usage: TokenUsage } = $props();
</script>

<dl class="my-5 grid grid-cols-3 gap-3 sm:max-w-3xl sm:gap-8" aria-label="Run usage">
  <div>
    <dt class="text-xs text-base-content/70">Total tokens</dt>
    <dd class="mt-1 text-xl font-semibold tracking-tight tabular-nums sm:text-3xl">
      {tokenCount(usage).toLocaleString()}
    </dd>
    <dd class="mt-1 text-xs text-base-content/65">
      {usage.input.toLocaleString()} in / {usage.output.toLocaleString()} out
    </dd>
  </div>
  <div>
    <dt class="text-xs text-base-content/70">Cached tokens</dt>
    <dd class="mt-1 text-xl font-semibold tracking-tight tabular-nums sm:text-3xl">
      {usage.cacheRead.toLocaleString()}
    </dd>
    <dd class="mt-1 text-xs text-base-content/65">
      Read / {usage.cacheWrite.toLocaleString()} written
    </dd>
  </div>
  <div>
    <dt class="text-xs text-base-content/70">Estimated cost</dt>
    <dd class="mt-1 text-xl font-semibold tracking-tight tabular-nums sm:text-3xl">
      {usage.cost === undefined ? 'Unknown' : `$${usage.cost.toFixed(4)}`}
    </dd>
    <dd class="mt-1 text-xs text-base-content/65">
      {usage.costIncomplete ? 'USD · partial estimate' : 'USD'}
    </dd>
  </div>
</dl>
{#if usage.costIncomplete}
  <p class="mb-3 text-xs text-base-content/70">Some tokens have no reported price.</p>
{/if}
