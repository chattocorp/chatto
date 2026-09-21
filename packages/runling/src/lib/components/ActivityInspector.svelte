<script lang="ts">
  import { onMount } from "svelte";
  import { activityStatus, type Activity } from "$lib/timeline.ts";
  import { duration } from "$lib/runs.ts";
  import AnsiText from "./AnsiText.svelte";
  import Usage from "./Usage.svelte";

  let { activity, elapsed, onclose }: { activity: Activity; elapsed: number; onclose: () => void } = $props();
  let dialog: HTMLDialogElement;
  const titleId = $props.id();

  onMount(() => {
    dialog.showModal();
  });
</script>

<dialog class="modal modal-middle" bind:this={dialog} {onclose} aria-labelledby={titleId}>
  <div class="modal-box flex max-h-[85dvh] w-11/12 max-w-4xl flex-col overflow-hidden p-0">
    <header class="flex shrink-0 items-start justify-between gap-4 border-b border-base-300 px-6 py-4">
      <div class="min-w-0 space-y-2">
        <h2 id={titleId} class="text-lg font-medium wrap-anywhere">{activity.label}</h2>
        <p class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-base-content/60">
          <span>{activity.kind}</span>
          <span>{activityStatus(activity)}</span>
          <span>Started at {duration(activity.startedAt)}</span>
          <span>{activity.kind === "input" ? "Wait" : "Duration"} {duration(activity.durationMs ?? Math.max(0, elapsed - activity.startedAt))}</span>
        </p>
      </div>
      <button class="btn btn-ghost btn-sm btn-square shrink-0 cursor-pointer" onclick={() => dialog.close()} aria-label="Close activity details">
        <span class="icon-[lucide--x] size-4" aria-hidden="true"></span>
      </button>
    </header>
    <div class="min-h-0 overflow-auto px-6 py-5">
      {#if activity.usage}<Usage usage={activity.usage} detail />{/if}
      <h3 class="mb-3 text-sm font-medium" class:mt-5={!!activity.usage}>Activity logs</h3>
      <pre class="m-0 whitespace-pre-wrap font-mono text-xs leading-relaxed wrap-anywhere"><AnsiText text={activity.logs.length ? activity.logs.join("\n\n") : "No logs for this activity."} /></pre>
    </div>
  </div>
  <form method="dialog" class="modal-backdrop">
    <button class="cursor-pointer" aria-label="Close activity details">Close</button>
  </form>
</dialog>
