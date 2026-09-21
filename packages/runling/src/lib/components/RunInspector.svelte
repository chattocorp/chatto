<script lang="ts">
  import { onMount } from "svelte";
  import type { RunDetail } from "$lib/runs.ts";
  import { summarizeRunActivity } from "$lib/run-activity.ts";
  import { duration } from "$lib/runs.ts";
  import { buildTimeline, findActivity } from "$lib/timeline.ts";
  import StatusBadge from "./StatusBadge.svelte";
  import Timeline from "./Timeline.svelte";
  import Usage from "./Usage.svelte";
  import RunValue from "./RunValue.svelte";
  import RunOutput from "./RunOutput.svelte";
  import ActivityInspector from "./ActivityInspector.svelte";
  let { run, connection }: { run: RunDetail; connection: string } = $props();
  let cancelling = $state(false);
  let cancelError = $state("");

  async function cancelRun() {
    if (cancelling) return;
    cancelling = true;
    cancelError = "";
    try {
      const response = await fetch(`/api/runs/${run.id}/cancel`, { method: "POST" });
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error ?? "Cannot cancel this run.");
      }
    } catch (cause) {
      cancelError = cause instanceof Error ? cause.message : "Cannot cancel this run.";
      cancelling = false;
    }
  }

  let now = $state(Date.now());
  let selected = $state("");
  let tab = $state<"timeline" | "input" | "output" | "logs">("timeline");
  let pendingInputs = $derived(summarizeRunActivity(run)?.pendingInputs ?? 0);
  let nodes = $derived(buildTimeline(run.events, run.status));
  let activity = $derived(findActivity(nodes, selected));
  let elapsed = $derived(run.durationMs ?? Math.max(0, now - run.startedAt));
  let logs = $derived(
    run.events.filter((e) => e.type === "log").map((e) => e.message),
  );
  onMount(() => {
    const timer = setInterval(() => {
      now = Date.now();
    }, 500);
    return () => clearInterval(timer);
  });
</script>

<section class="min-w-0" aria-label="Run details">
  <header class="px-8 pt-8 pb-4 max-sm:p-5">
    <div class="flex min-h-8 items-center justify-between gap-3">
      <span class="min-w-0 wrap-anywhere text-base-content/60 text-xs"
        >{run.source === "web"
          ? "Started from web"
          : `Webhook /${run.webhook}`}</span
      >
      <div class="flex shrink-0 items-center gap-2">
        <StatusBadge status={run.status} waiting={pendingInputs > 0} />
        {#if run.status === "running"}
          <button
            class="btn btn-ghost btn-sm min-w-28 gap-1.5 px-2 text-xs font-medium text-base-content/60 hover:bg-error/10 hover:text-error focus-visible:outline-error focus-visible:text-error"
            disabled={cancelling}
            onclick={cancelRun}
          >
            <span
              class={cancelling
                ? "icon-[lucide--loader-circle] size-3.5 animate-spin motion-reduce:animate-none"
                : "icon-[lucide--square] size-3.5"}
              aria-hidden="true"
            ></span>
            {cancelling ? "Cancelling…" : "Cancel run"}
          </button>
          <span class="sr-only" role="status">{cancelling ? "Waiting for the workflow to stop." : ""}</span>
        {/if}
      </div>
    </div>
    <h1 class="text-3xl tracking-tight font-medium my-3.5 mx-0 wrap-anywhere">
      {run.workflow}
    </h1>
    <div
      class="flex flex-wrap gap-y-2.5 gap-x-5.5 text-base-content/60 text-xs tabular-nums"
    >
      <span>{new Date(run.startedAt).toLocaleString()}</span><span
        >{duration(elapsed)}</span
      >
    </div>
    {#if pendingInputs}<p class="mt-3 text-sm text-warning">{pendingInputs} {pendingInputs === 1 ? "input" : "inputs"} pending</p>{/if}
    {#if run.status === "running" && cancelError}
      <p class="mt-3 text-sm text-error" role="alert">{cancelError}</p>
    {/if}
    <Usage usage={run.usage} detail />
    <p
      class="text-base-content/60 text-xs flex justify-between mt-5 mr-0 mb-0 ml-0"
    >
      Run {run.id.slice(0, 8)} <span>{connection}</span>
    </p>
  </header>
  {#if run.status === "cancelled"}
    <p class="mx-8 mb-2.5 flex items-center gap-2 text-sm text-base-content/60 max-sm:mx-5" role="status">
      <span class="icon-[lucide--circle-stop] size-4 shrink-0" aria-hidden="true"></span>
      Run cancelled
    </p>
  {:else if run.error}<div
      class="alert alert-error mt-0 mr-8 mb-2.5 ml-8 max-sm:mx-5"
      role="alert"
    >
      {run.error}
    </div>{/if}
  <nav
    class="tabs tabs-border px-8 border-b border-base-300 max-sm:px-5"
    aria-label="Run views"
  >
    <button
      class="tab gap-2 text-sm"
      class:tab-active={tab === "timeline"}
      onclick={() => (tab = "timeline")}
      >Timeline <span class="badge badge-sm badge-ghost text-xs"
        >{run.events.filter((e) => e.type === "step.started").length}</span
      ></button
    >
    <button
      class="tab gap-2 text-sm"
      class:tab-active={tab === "input"}
      onclick={() => (tab = "input")}>Input</button
    >
    <button
      class="tab gap-2 text-sm"
      class:tab-active={tab === "output"}
      onclick={() => (tab = "output")}>Output</button
    >
    <button
      class="tab gap-2 text-sm"
      class:tab-active={tab === "logs"}
      onclick={() => (tab = "logs")}
      >Logs <span class="badge badge-sm badge-ghost text-xs">{logs.length}</span
      ></button
    >
  </nav>
  <div class="py-6 px-8 max-sm:p-5">
    {#if tab === "timeline"}
      <div class="flex justify-between text-base-content/60 text-xs mb-3.5">
        <span>Execution timeline</span><span>0 → {duration(elapsed)}</span>
      </div>
      {#if nodes.length}
        <Timeline
          {nodes}
          {selected}
          onselect={(id) => (selected = selected === id ? "" : id)}
          {elapsed}
          running={run.status === "running"}
        />
        <p class="mt-3 text-base-content/60 text-xs leading-relaxed">
          Select a block to inspect its activity. Collapse a step to focus the
          timeline.
        </p>
      {:else}<div
          class="border border-base-300 border-dashed py-9 px-5 text-base-content/60 rounded-lg text-sm text-center"
        >
          {run.status === "running"
            ? "Waiting for the first workflow event…"
            : "No activity events were recorded for this run."}
        </div>{/if}
      {#if activity}
        <ActivityInspector {activity} {elapsed} onclose={() => (selected = "")} />
      {/if}
    {:else if tab === "input"}
      {#key run.id}<RunValue value={run.input} kind="input" />{/key}
    {:else if tab === "output"}
      {#key run.id}<RunOutput {run} />{/key}
    {:else}
      {#key run.id}<RunValue value={logs.join("\n\n")} kind="logs" />{/key}
    {/if}
  </div>
</section>
