<script lang="ts">
  import ColumnResizer from "$lib/components/ColumnResizer.svelte";
  import ConnectionOverlay from "$lib/components/ConnectionOverlay.svelte";
  import ThemePicker from "$lib/components/ThemePicker.svelte";
  import SidebarToggle from "$lib/components/SidebarToggle.svelte";
  import { isSidebarShortcut } from "$lib/sidebar-shortcut.ts";
  import { invalidateAll } from "$app/navigation";
  import { onMount } from "svelte";
  import type { PageData } from "./$types";
  import {
    applyRecord,
    type RunDetail,
    type RunRecord,
    type RunSummary,
    type WebhookInfo,
  } from "$lib/runs.ts";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import { isRunWaiting } from "$lib/run-activity.ts";
  import RunInspector from "$lib/components/RunInspector.svelte";
  import RunComposer from "$lib/components/RunComposer.svelte";
  import WebhookInfoModal from "$lib/components/WebhookInfo.svelte";

  let { data }: { data: PageData } = $props();
  let liveRuns = $state<RunSummary[] | null>(null);
  let runs = $derived(liveRuns ?? data.runs);
  let filter = $state("");
  let visibleRuns = $derived(runs.filter((run) => !filter || run.webhook === filter));
  let selectedHook = $derived(data.webhooks.find((hook) => hook.name === filter));
  let selected = $state("");
  let detail = $state<RunDetail | null>(null);
  let connection = $state("Connecting…");
  let detailError = $state("");
  let listConnected = $state(false);
  let composer = $state(false);
  let hookInfo = $state<WebhookInfo | null>(null);
  let configError = $state("");
  let runsWidth = $state<number>();
  let runsExpanded = $state(true);
  let runStream: EventSource | undefined;

  function handleSidebarShortcut(event: KeyboardEvent) {
    if (!listConnected || !isSidebarShortcut(event)) return;
    const target = event.target;
    if (target instanceof HTMLElement &&
      (target.isContentEditable || target.closest("input, textarea, select, [role='textbox']"))) return;
    event.preventDefault();
    if (!event.repeat) toggleRuns();
  }

  function toggleRuns() {
    runsExpanded = !runsExpanded;
    try {
      localStorage.setItem("runling-runs-sidebar", runsExpanded ? "expanded" : "collapsed");
    } catch {
      // Layout controls also work when browser storage is unavailable.
    }
  }

  async function selectRun(id: string) {
    runStream?.close();
    selected = id;
    detail = null;
    detailError = "";
    connection = "Connecting…";
    const url = new URL(location.href);
    url.searchParams.set("run", id);
    history.replaceState(null, "", url);
    try {
      const response = await fetch(`/api/runs/${encodeURIComponent(id)}`);
      if (!response.ok)
        throw new Error(response.status === 404 ? "Run not found." : "Could not load this run.");
      const saved = (await response.json()) as RunDetail;
      if (selected !== id) return;
      detail = saved;
      if (saved.status !== "running") {
        connection = "Saved";
        return;
      }
    } catch (cause) {
      if (selected === id) detailError = cause instanceof Error ? cause.message : String(cause);
      return;
    }
    const stream = new EventSource(`/api/runs/${encodeURIComponent(id)}/events`);
    runStream = stream;
    stream.addEventListener("snapshot", (event) => {
      if (selected !== id) return;
      detail = JSON.parse(event.data) as RunDetail;
      connection = detail.status === "running" ? "Live" : "Saved";
      if (detail.status !== "running") stream.close();
    });
    stream.addEventListener("record", (event) => {
      if (!detail || selected !== id) return;
      detail = applyRecord(detail, JSON.parse(event.data) as RunRecord);
      if (detail.status !== "running") {
        connection = "Saved";
        stream.close();
      }
    });
    stream.onopen = () => { connection = "Live"; };
    stream.onerror = () => { connection = "Connection lost. Retrying…"; };
  }

  onMount(() => {
    try {
      const saved = localStorage.getItem("runling-runs-sidebar") ?? localStorage.getItem("runling-sidebars");
      runsExpanded = saved !== "collapsed";
    } catch {
      // Keep the default layout when browser storage is unavailable.
    }
    const configs = new EventSource("/api/config/events");
    let revision: number | undefined;
    configs.onopen = () => { revision = undefined; };
    configs.addEventListener("config", (event) => {
      const update = JSON.parse(event.data) as { revision: number; error: string | null };
      configError = update.error ?? "";
      if (revision !== update.revision) {
        revision = update.revision;
        composer = false;
        hookInfo = null;
        void invalidateAll();
      }
    });
    const stream = new EventSource("/api/runs/events");
    stream.addEventListener("runs", (event) => {
      liveRuns = JSON.parse(event.data);
      listConnected = true;
    });
    stream.onerror = () => { listConnected = false; };
    const initial = new URL(location.href).searchParams.get("run") ?? data.runs[0]?.id;
    if (initial) void selectRun(initial);
    return () => {
      configs.close();
      stream.close();
      runStream?.close();
    };
  });
</script>

<svelte:window onkeydown={handleSidebarShortcut} />

<svelte:head>
  <title>Runling — Runs</title>
  <meta name="description" content="Start workflows and inspect their live execution." />
</svelte:head>

<div class="flex h-dvh min-h-0 flex-col overflow-hidden bg-base-100 text-base-content">
  {#if configError}
    <div class="alert alert-error shrink-0 rounded-none" role="alert">Configuration reload failed. The last valid configuration is still active. {configError}</div>
  {/if}
  <header class="navbar shrink-0 gap-3 border-b border-base-300 bg-base-200 px-3 sm:px-4">
    <div class="hidden md:block"><SidebarToggle expanded={runsExpanded} onclick={toggleRuns} /></div>
    <a href="/" class="flex items-center gap-2 text-lg font-semibold tracking-tight" aria-label="Runling home">
      <svg class="size-6 text-primary" viewBox="0 0 28 28" fill="none" aria-hidden="true">
        <path d="M3 25V12l7 4V9l7 4V3h7v22H3Z" fill="currentColor" />
        <path d="M7 21h3m4 0h3m3 0h2" class="stroke-primary-content" stroke-width="2" />
      </svg>
      runling
    </a>
    <div class="ml-auto flex items-center gap-2 sm:gap-3">
      <ThemePicker />
      <span class="hidden items-center gap-2 text-xs text-base-content/60 sm:flex" role="status">
        <span class="status" class:status-success={listConnected} class:status-warning={!listConnected}></span>
        {listConnected ? "Connected" : "Reconnecting"}
      </span>
      <button class="btn btn-primary btn-sm" disabled={!data.webhooks.length} onclick={() => (composer = true)}>+ New run</button>
    </div>
  </header>

  <div class={["flex min-h-0 flex-1 flex-col md:grid", runsExpanded ? "md:grid-cols-[var(--runs-width)_minmax(0,1fr)]" : "md:grid-cols-1"]} style:--runs-width={runsWidth === undefined ? "17rem" : `${runsWidth}px`}>
    <aside
      class="relative hidden min-h-0 min-w-0 flex-col border-r border-base-300 bg-base-200 md:flex [&[hidden]]:hidden"
      id="runs-sidebar"
      hidden={!runsExpanded}
      aria-label="Workflow runs"
    >
      <div class="shrink-0 border-b border-base-300 px-3 py-3">
        <div class="mb-2 flex items-center justify-between gap-2">
          <h2 class="m-0 text-sm font-semibold">Runs</h2>
          <span class="text-xs text-base-content/50 tabular-nums">{visibleRuns.length}</span>
        </div>
        <div class="flex items-center gap-1">
          <label class="sr-only" for="desktop-webhook-filter">Filter runs by webhook</label>
          <select id="desktop-webhook-filter" class="select select-sm min-w-0 flex-1" bind:value={filter}>
            <option value="">All webhooks</option>
            {#each data.webhooks as webhook (webhook.name)}<option value={webhook.name}>{webhook.name}</option>{/each}
          </select>
          {#if selectedHook}
            <button class="btn btn-ghost btn-square btn-sm" onclick={() => (hookInfo = selectedHook)} aria-label={`Information about ${selectedHook.name}`} title="Webhook information">
              <span class="icon-[lucide--info] size-4" aria-hidden="true"></span>
            </button>
          {/if}
        </div>
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <ul class="m-0 list-none p-1.5">
          {#each visibleRuns as run (run.id)}
            <li>
              <button
                class={[
                  "w-full rounded-md px-3 py-2.5 text-left hover:bg-base-300/70 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary",
                  selected === run.id && "bg-base-300",
                ]}
                onclick={() => selectRun(run.id)}
                aria-pressed={selected === run.id}
                title={`Run ${run.id} · ${new Date(run.startedAt).toLocaleString()}`}
              >
                <span class="flex min-w-0 items-center gap-2">
                  <StatusBadge status={run.status} waiting={isRunWaiting(run.activity)} />
                  <span class="min-w-0 flex-1 truncate text-sm font-medium">{run.workflow}</span>
                </span>
                <span class="mt-1 flex min-w-0 items-center justify-between gap-2 pl-1 text-xs text-base-content/50">
                  <span class="truncate">{run.reference ?? run.id}</span>
                  <time class="shrink-0 tabular-nums" datetime={new Date(run.startedAt).toISOString()}>{new Date(run.startedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>
                </span>
              </button>
            </li>
          {:else}
            <li class="px-3 py-5 text-sm text-base-content/60">{filter ? "No runs for this webhook." : "No runs yet. Start a run to see its log here."}</li>
          {/each}
        </ul>
      </div>
      <ColumnResizer bind:width={runsWidth} storageKey="runling-width-runs" label="Runs column width" minWidth={180} class="hidden md:block" />
    </aside>

    <div class="flex shrink-0 items-center gap-2 border-b border-base-300 bg-base-200 px-3 py-2 md:hidden">
      <label class="sr-only" for="mobile-webhook-filter">Filter runs by webhook</label>
      <select id="mobile-webhook-filter" class="select select-sm w-28 shrink-0" bind:value={filter}>
        <option value="">All webhooks</option>
        {#each data.webhooks as webhook (webhook.name)}<option value={webhook.name}>{webhook.name}</option>{/each}
      </select>
      <label class="sr-only" for="mobile-run-picker">Select a run</label>
      <select id="mobile-run-picker" class="select select-sm min-w-0 flex-1" value={visibleRuns.some((run) => run.id === selected) ? selected : ""} onchange={(event) => { if (event.currentTarget.value) void selectRun(event.currentTarget.value); }}>
        <option value="">Select a run</option>
        {#each visibleRuns as run (run.id)}<option value={run.id}>{run.workflow} · {run.status} · {run.reference ?? run.id}</option>{/each}
      </select>
      {#if selectedHook}
        <button class="btn btn-ghost btn-square btn-sm shrink-0" onclick={() => (hookInfo = selectedHook)} aria-label={`Information about ${selectedHook.name}`} title="Webhook information">
          <span class="icon-[lucide--info] size-4" aria-hidden="true"></span>
        </button>
      {/if}
    </div>

    <main class="min-h-0 min-w-0 flex-1 overflow-hidden bg-base-100">
      {#if detail}
        {#key detail.id}<RunInspector run={detail} {connection} />{/key}
      {:else if selected}
        <div class="hero h-full p-8">
          <div class="hero-content flex-col text-center">
            {#if !detailError}<span class="loading loading-spinner loading-lg text-primary"></span>{/if}
            <h1 class="text-2xl font-semibold">{detailError ? "Run unavailable" : "Loading run"}</h1>
            <p class="text-base-content/60">{detailError || connection}</p>
            {#if detailError}<button class="btn btn-sm" onclick={() => selectRun(selected)}>Retry</button>{/if}
          </div>
        </div>
      {:else}
        <div class="hero h-full p-8">
          <div class="hero-content flex-col text-center">
            <h1 class="text-2xl font-semibold tracking-tight">No run selected</h1>
            <p class="max-w-sm text-base-content/60">Select a run to read its log, or start a new run.</p>
            <button class="btn btn-primary" disabled={!data.webhooks.length} onclick={() => (composer = true)}>Start a run</button>
          </div>
        </div>
      {/if}
    </main>
  </div>
</div>

{#if hookInfo}<WebhookInfoModal webhook={hookInfo} onclose={() => (hookInfo = null)} />{/if}
{#if composer}<RunComposer
    webhooks={data.webhooks}
    initialWebhook={filter}
    onclose={() => (composer = false)}
    onstarted={(id) => {
      filter = "";
      void selectRun(id);
    }}
  />{/if}
{#if !listConnected}<ConnectionOverlay />{/if}
