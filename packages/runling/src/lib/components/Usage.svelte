<script lang="ts">
  import type { TokenUsage } from "runling";
  import { estimatedCost, tokenCount } from "$lib/usage.ts";
  let {
    usage,
    detail = false,
    compact = false,
  }: { usage: TokenUsage; detail?: boolean; compact?: boolean } = $props();
</script>

<span
  class={[
    "text-xs text-base-content/60 tabular-nums",
    compact
      ? "flex min-w-0 gap-2 whitespace-nowrap"
      : "inline-flex flex-wrap gap-x-3 gap-y-1",
  ]}
  title={`${tokenCount(usage).toLocaleString()} tokens · ${estimatedCost(usage)}. Estimated model cost in USD, not an invoice. Totals include child activities.`}
>
  <span class="min-w-0 truncate"
    >{tokenCount(usage).toLocaleString()} tokens</span
  >
  <span class="min-w-0 truncate">{estimatedCost(usage)}</span>
</span>
{#if detail}
  <dl class="flex flex-wrap gap-y-3 gap-x-6 my-3 mx-0 text-xs">
    <div>
      <dt class="text-base-content/60 text-xs">Input</dt>
      <dd class="mt-1 mr-0 mb-0 ml-0 tabular-nums">
        {usage.input.toLocaleString()}
      </dd>
    </div>
    <div>
      <dt class="text-base-content/60 text-xs">Output</dt>
      <dd class="mt-1 mr-0 mb-0 ml-0 tabular-nums">
        {usage.output.toLocaleString()}
      </dd>
    </div>
    <div>
      <dt class="text-base-content/60 text-xs">Cache read</dt>
      <dd class="mt-1 mr-0 mb-0 ml-0 tabular-nums">
        {usage.cacheRead.toLocaleString()}
      </dd>
    </div>
    <div>
      <dt class="text-base-content/60 text-xs">Cache write</dt>
      <dd class="mt-1 mr-0 mb-0 ml-0 tabular-nums">
        {usage.cacheWrite.toLocaleString()}
      </dd>
    </div>
  </dl>
  {#if usage.costIncomplete}<p class="text-base-content/60 text-xs">
      Some tokens have no reported price. The estimate includes only known
      costs.
    </p>{/if}
{/if}
