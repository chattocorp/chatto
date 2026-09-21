<script lang="ts">
  import type { RunActivity } from "$lib/runs.ts";
  let { activity }: { activity?: RunActivity | null } = $props();
  let label = $derived(activity?.waiting ? `${activity.pendingInputs ?? 1} ${(activity.pendingInputs ?? 1) === 1 ? "input" : "inputs"} pending` : activity?.step ?? activity?.label ?? "Waiting for activity…");
  let detail = $derived(activity?.preview ?? (activity?.label !== label ? activity?.label : undefined));
</script>

<span class="my-2 block min-w-0 border-l border-base-300 pl-2.5 text-xs leading-5">
  <span class="flex min-w-0 items-center gap-1.5">
    <span class={activity?.waiting ? "truncate text-warning" : "truncate text-base-content/80"} title={label}>{label}</span>
    {#if activity?.parallel}<span class="shrink-0 text-base-content/40" title={`${activity.parallel} other activities running`}>{activity.waiting ? `${activity.parallel} active` : `+${activity.parallel}`}</span>{/if}
  </span>
  {#if detail}<span class="block truncate text-base-content/50" title={detail}>{detail}</span>{/if}
</span>
