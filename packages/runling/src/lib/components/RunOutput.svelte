<script lang="ts">
  import type { RunDetail } from "$lib/runs.ts";
  import RunValue from "./RunValue.svelte";

  let { run }: { run: RunDetail } = $props();
</script>

{#if run.status === "running" || run.output === null}
  <section class="flex items-start gap-3 rounded-lg border border-dashed border-base-300 p-6" aria-label="Workflow output">
    <span
      class={run.status === "running" ? "loading loading-spinner loading-sm text-base-content/50" : "icon-[lucide--file-text] size-5 shrink-0 text-base-content/50"}
      aria-hidden="true"
    ></span>
    <div class="space-y-1">
      <h2 class="text-sm font-medium">{run.status === "running" ? "Waiting for output" : "No output returned"}</h2>
      <p class="text-xs leading-relaxed text-base-content/60">
        {run.status === "running"
          ? "The result will appear here when the workflow finishes. Follow its progress in Timeline or Logs."
          : "This run did not return a result. Check Timeline or Logs for activity details."}
      </p>
    </div>
  </section>
{:else}
  <RunValue value={run.output} kind="output" />
{/if}
