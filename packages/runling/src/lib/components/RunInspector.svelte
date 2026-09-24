<script lang="ts">
  import { onMount } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import type { RunDetail } from '$lib/runs.ts';
  import { isRunWaiting, summarizeRunActivity } from '$lib/run-activity.ts';
  import { duration } from '$lib/runs.ts';
  import { runLogRows } from '$lib/run-log.ts';
  import { buildTimeline, findActivity } from '$lib/timeline.ts';
  import StatusBadge from './StatusBadge.svelte';
  import Timeline from './Timeline.svelte';
  import Usage from './Usage.svelte';
  import RunValue from './RunValue.svelte';
  import RunOutput from './RunOutput.svelte';
  import RunLog from './RunLog.svelte';
  import ActivityInspector from './ActivityInspector.svelte';
  import ActivityDetails from './ActivityDetails.svelte';

  let { run, connection }: { run: RunDetail; connection: string } = $props();
  let cancelling = $state(false);
  let cancelError = $state('');
  let reference = $derived(run.reference ?? run.id);
  let copiedReference = $state('');
  let failedReference = $state('');
  let now = $state(Date.now());
  let selected = $state('');
  let tab = $state<'log' | 'timeline'>('log');
  let currentActivity = $derived(summarizeRunActivity(run));
  let pendingInputs = $derived(currentActivity?.pendingInputs ?? 0);
  let nodes = $derived(buildTimeline(run.events, run.status));
  let activity = $derived(findActivity(nodes, selected));
  let elapsed = $derived(run.durationMs ?? Math.max(0, now - run.startedAt));
  let quietFor = $derived(Math.max(0, elapsed - (run.events.at(-1)?.timestamp ?? 0)));
  let logs = $derived(runLogRows(run.events));
  const wide = new MediaQuery('(min-width: 96rem)', false);
  const paneTitleId = $props.id();

  function handleTabKey(event: KeyboardEvent) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    tab =
      event.key === 'Home'
        ? 'log'
        : event.key === 'End'
          ? 'timeline'
          : tab === 'log'
            ? 'timeline'
            : 'log';
    const tabs =
      event.currentTarget instanceof HTMLElement
        ? event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')
        : undefined;
    tabs?.[tab === 'log' ? 0 : 1]?.focus();
  }

  async function copyReference() {
    const value = reference;
    try {
      await navigator.clipboard.writeText(value);
      copiedReference = value;
      failedReference = '';
    } catch {
      failedReference = value;
    }
  }

  async function cancelRun() {
    if (cancelling) return;
    cancelling = true;
    cancelError = '';
    try {
      const response = await fetch(`/api/runs/${run.id}/cancel`, { method: 'POST' });
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error ?? 'Cannot cancel this run.');
      }
    } catch (cause) {
      cancelError = cause instanceof Error ? cause.message : 'Cannot cancel this run.';
      cancelling = false;
    }
  }

  onMount(() => {
    const timer = setInterval(() => {
      now = Date.now();
    }, 500);
    return () => clearInterval(timer);
  });
</script>

<section class="flex h-full min-h-0 min-w-0 flex-col" aria-label="Run details">
  <header class="shrink-0 border-b border-base-300 px-4 pt-4 pb-3 sm:px-6">
    <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
      <div class="min-w-0 flex-1">
        <h1 class="m-0 wrap-anywhere text-xl font-semibold tracking-tight sm:text-2xl">
          {run.workflow}
        </h1>
        <div
          class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-base-content/55 tabular-nums"
        >
          <span
            >{run.source === 'web'
              ? 'Started from web'
              : run.source === 'source'
                ? `Source ${run.sourceName ?? run.webhook}`
                : `Webhook /${run.webhook}`}</span
          >
          <time datetime={new Date(run.startedAt).toISOString()}
            >{new Date(run.startedAt).toLocaleString()}</time
          >
          <span>{duration(elapsed)}</span>
          <span title={connection}>{connection}</span>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <StatusBadge status={run.status} waiting={isRunWaiting(currentActivity)} />
        {#if run.status === 'running'}
          <button
            class="btn btn-ghost btn-sm gap-1.5 text-xs text-base-content/65 hover:bg-error/10 hover:text-error focus-visible:outline-error"
            disabled={cancelling}
            onclick={cancelRun}
          >
            <span
              class={cancelling
                ? 'icon-[lucide--loader-circle] size-3.5 animate-spin motion-reduce:animate-none'
                : 'icon-[lucide--square] size-3.5'}
              aria-hidden="true"
            ></span>
            {cancelling ? 'Cancelling…' : 'Cancel run'}
          </button>
          <span class="sr-only" role="status"
            >{cancelling ? 'Waiting for the workflow to stop.' : ''}</span
          >
        {/if}
      </div>
    </div>
    {#if pendingInputs}<p class="mt-2 text-sm text-warning">
        {pendingInputs}
        {pendingInputs === 1 ? 'input' : 'inputs'} pending
      </p>{/if}
    {#if run.status === 'running' && !isRunWaiting(currentActivity) && quietFor >= 120_000}
      <p class="mt-2 text-sm text-warning" role="status">
        No recorded activity for {duration(quietFor)}. Last reported status: running.
      </p>
    {/if}
    {#if cancelError}<p class="mt-2 text-sm text-error" role="alert">{cancelError}</p>{/if}
    {#if run.status === 'cancelled'}
      <p class="mt-2 text-sm text-base-content/60" role="status">Run cancelled</p>
    {:else if run.error}
      <p class="mt-2 text-sm text-error" role="alert">{run.error}</p>
    {/if}
    <div class="mt-2 flex min-w-0 items-center gap-1 text-xs text-base-content/60">
      <span class="select-text truncate" title={reference}>{reference}</span>
      <button
        class="btn btn-ghost btn-square btn-xs shrink-0"
        onclick={copyReference}
        aria-label="Copy run reference"
        title="Copy run reference"
      >
        <span
          class={copiedReference === reference
            ? 'icon-[lucide--check] size-3.5'
            : 'icon-[lucide--copy] size-3.5'}
          aria-hidden="true"
        ></span>
      </button>
    </div>
    <p class="sr-only" role="status">
      {copiedReference === reference ? 'Run reference copied.' : ''}
    </p>
    {#if failedReference === reference}<p class="text-xs text-error" role="alert">
        Could not copy. Select the reference and copy it manually.
      </p>{/if}
    <details class="mt-2 text-sm 2xl:hidden">
      <summary
        class="w-fit cursor-pointer text-xs text-base-content/65 hover:text-base-content focus-visible:outline-2 focus-visible:outline-primary"
        >Run details</summary
      >
      <div class="mt-3 max-h-[40vh] space-y-4 overflow-auto pb-2">
        <Usage usage={run.usage} detail />
        <div class="grid gap-4 xl:grid-cols-2">
          <RunValue value={run.input} kind="input" />
          <RunOutput {run} />
        </div>
      </div>
    </details>
  </header>

  <div class="flex min-h-0 min-w-0 flex-1">
    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      <div
        class="tabs tabs-border shrink-0 border-b border-base-300 px-4 sm:px-6"
        aria-label="Run views"
        role="tablist"
      >
        <button
          class="tab text-sm"
          class:tab-active={tab === 'log'}
          role="tab"
          aria-selected={tab === 'log'}
          aria-controls="run-log-panel"
          tabindex={tab === 'log' ? 0 : -1}
          onclick={() => (tab = 'log')}
          onkeydown={handleTabKey}>Log</button
        >
        <button
          class="tab text-sm"
          class:tab-active={tab === 'timeline'}
          role="tab"
          aria-selected={tab === 'timeline'}
          aria-controls="run-timeline-panel"
          tabindex={tab === 'timeline' ? 0 : -1}
          onclick={() => (tab = 'timeline')}
          onkeydown={handleTabKey}>Timeline</button
        >
      </div>
      {#if tab === 'log'}
        <div
          id="run-log-panel"
          class="flex min-h-0 flex-1 flex-col"
          role="tabpanel"
          aria-label="Log"
        >
          <RunLog
            rows={logs}
            running={run.status === 'running'}
            selectedActivity={selected}
            onselectActivity={(id) => (selected = id)}
          />
        </div>
      {:else}
        <div
          id="run-timeline-panel"
          class="min-h-0 flex-1 overflow-auto px-4 py-5 sm:px-6"
          role="tabpanel"
          aria-label="Timeline"
        >
          <div class="mb-3 flex justify-between text-xs text-base-content/60">
            <span>Execution timeline</span><span>0 → {duration(elapsed)}</span>
          </div>
          {#if nodes.length}
            <Timeline
              {nodes}
              {selected}
              onselect={(id) => (selected = selected === id ? '' : id)}
              {elapsed}
              running={run.status === 'running'}
            />
            <p class="mt-3 text-xs leading-relaxed text-base-content/60">
              Select a block to inspect its activity. Collapse a step to focus the timeline.
            </p>
          {:else}
            <div
              class="rounded-lg border border-dashed border-base-300 px-5 py-9 text-center text-sm text-base-content/60"
            >
              {run.status === 'running'
                ? 'Waiting for the first workflow event…'
                : 'No activity events were recorded for this run.'}
            </div>
          {/if}
        </div>
      {/if}
    </div>
    <aside
      class="hidden min-h-0 min-w-[25rem] w-[40%] max-w-[48rem] flex-col border-l border-base-300 2xl:flex"
      aria-label="Details pane"
    >
      {#if activity}
        <ActivityDetails
          {activity}
          {elapsed}
          titleId={paneTitleId}
          onclose={() => (selected = '')}
        />
      {:else}
        <div class="shrink-0 border-b border-base-300 px-5 py-4">
          <h2 class="text-lg font-medium">Run details</h2>
        </div>
        <div class="min-h-0 space-y-5 overflow-auto px-5 py-5">
          <Usage usage={run.usage} detail />
          <RunValue value={run.input} kind="input" />
          <RunOutput {run} />
        </div>
      {/if}
    </aside>
  </div>
  {#if activity && !wide.current}<ActivityInspector
      {activity}
      {elapsed}
      onclose={() => (selected = '')}
    />{/if}
</section>
