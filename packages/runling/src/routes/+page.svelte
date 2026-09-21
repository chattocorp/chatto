<script lang="ts">
  import ColumnResizer from "$lib/components/ColumnResizer.svelte";
  import ConnectionOverlay from "$lib/components/ConnectionOverlay.svelte";
  import ThemePicker from "$lib/components/ThemePicker.svelte";
  import SidebarToggle from "$lib/components/SidebarToggle.svelte";
  import { isSidebarShortcut } from "$lib/sidebar-shortcut.ts";
  import Usage from "$lib/components/Usage.svelte";
  import { invalidateAll } from "$app/navigation";
  import { onMount } from "svelte";
  import type { PageData } from "./$types";
  import {
    applyRecord,
    duration,
    type RunDetail,
    type RunRecord,
    type RunSummary,
    type WebhookInfo,
  } from "$lib/runs.ts";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import RunActivityPreview from "$lib/components/RunActivityPreview.svelte";
  import ActivityIndicator from "$lib/components/ActivityIndicator.svelte";
  import RunInspector from "$lib/components/RunInspector.svelte";
  import RunComposer from "$lib/components/RunComposer.svelte";
  import WebhookInfoModal from "$lib/components/WebhookInfo.svelte";

  let { data }: { data: PageData } = $props();
  let liveRuns = $state<RunSummary[] | null>(null);
  let runs = $derived(liveRuns ?? data.runs);
  let filter = $state("");
  let visibleRuns = $derived(
    runs.filter((run) => !filter || run.webhook === filter),
  );
  let selected = $state("");
  let detail = $state<RunDetail | null>(null);
  let connection = $state("Connecting…");
  let detailError = $state("");
  let listConnected = $state(false);
  let composer = $state<WebhookInfo | null>(null);
  let hookInfo = $state<WebhookInfo | null>(null);
  let copied = $state("");
  let notice = $state("");
  let configError = $state("");
  let webhookWidth = $state<number>();
  let runsWidth = $state<number>();
  let sidebarsExpanded = $state(true);

  function handleSidebarShortcut(event: KeyboardEvent) {
    if (!listConnected || !isSidebarShortcut(event)) return;
    const target = event.target;
    if (
      target instanceof HTMLElement &&
      (target.isContentEditable ||
        target.closest("input, textarea, select, [role='textbox']"))
    )
      return;
    event.preventDefault();
    if (!event.repeat) toggleSidebars();
  }

  function toggleSidebars() {
    sidebarsExpanded = !sidebarsExpanded;
    try {
      localStorage.setItem(
        "runling-sidebars",
        sidebarsExpanded ? "expanded" : "collapsed",
      );
    } catch {
      // Layout controls also work when browser storage is unavailable.
    }
  }
  let runStream: EventSource | undefined;
  let copyTimer: ReturnType<typeof setTimeout> | undefined;

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
        throw new Error(
          response.status === 404
            ? "Run not found."
            : "Could not load this run.",
        );
      const saved = (await response.json()) as RunDetail;
      if (selected !== id) return;
      detail = saved;
      if (saved.status !== "running") {
        connection = "Saved";
        return;
      }
    } catch (cause) {
      if (selected === id)
        detailError = cause instanceof Error ? cause.message : String(cause);
      return;
    }
    const stream = new EventSource(
      `/api/runs/${encodeURIComponent(id)}/events`,
    );
    runStream = stream;
    stream.addEventListener("snapshot", (event) => {
      if (selected !== id) return;
      detail = JSON.parse(event.data) as RunDetail;
      connection = detail.status === "running" ? "Live" : "Saved";
      if (detail.status !== "running") stream.close();
    });
    stream.addEventListener("record", (event) => {
      if (!detail || selected !== id) return;
      const record = JSON.parse(event.data) as RunRecord;
      detail = applyRecord(detail, record);
      if (detail.status !== "running") {
        connection = "Saved";
        stream.close();
      }
    });
    stream.onopen = () => {
      connection = "Live";
    };
    stream.onerror = () => {
      connection = "Connection lost. Retrying…";
    };
  }

  async function copyUrl(webhook: WebhookInfo) {
    notice = "";
    try {
      await navigator.clipboard.writeText(`${location.origin}${webhook.path}`);
      copied = webhook.name;
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => {
        copied = "";
      }, 2200);
    } catch {
      notice = `Copy this URL: ${location.origin}${webhook.path}`;
    }
  }

  onMount(() => {
    try {
      sidebarsExpanded =
        localStorage.getItem("runling-sidebars") !== "collapsed";
    } catch {
      // Keep the default layout when browser storage is unavailable.
    }
    const configs = new EventSource("/api/config/events");
    let revision: number | undefined;
    configs.onopen = () => {
      revision = undefined;
    };
    configs.addEventListener("config", (event) => {
      const update = JSON.parse(event.data) as {
        revision: number;
        error: string | null;
      };
      configError = update.error ?? "";
      if (revision !== update.revision) {
        revision = update.revision;
        composer = null;
        hookInfo = null;
        void invalidateAll();
      }
    });
    const stream = new EventSource("/api/runs/events");
    stream.addEventListener("runs", (event) => {
      liveRuns = JSON.parse(event.data);
      listConnected = true;
    });
    stream.onerror = () => {
      listConnected = false;
    };
    const initial =
      new URL(location.href).searchParams.get("run") ?? data.runs[0]?.id;
    if (initial) selectRun(initial);
    return () => {
      configs.close();
      stream.close();
      runStream?.close();
      clearTimeout(copyTimer);
    };
  });
</script>

<svelte:window onkeydown={handleSidebarShortcut} />

<svelte:head>
  <title>Runling — Runs</title>
  <meta
    name="description"
    content="Start workflows and inspect their live execution."
  />
</svelte:head>

<div
  class="flex min-h-screen flex-col bg-base-100 text-base-content lg:h-screen lg:overflow-hidden"
>
  {#if configError}
    <div class="alert alert-error rounded-none" role="alert">
      Configuration reload failed. The last valid configuration is still active. {configError}
    </div>
  {/if}
  <header
    class="navbar shrink-0 gap-3 border-b border-base-300 bg-base-200 px-4"
  >
    <SidebarToggle expanded={sidebarsExpanded} onclick={toggleSidebars} />
    <a
      href="/"
      class="flex items-center gap-2 text-xl font-semibold tracking-tight"
      aria-label="Runling home"
    >
      <svg
        class="size-7 text-primary"
        viewBox="0 0 28 28"
        fill="none"
        aria-hidden="true"
      >
        <path d="M3 25V12l7 4V9l7 4V3h7v22H3Z" fill="currentColor" />
        <path
          d="M7 21h3m4 0h3m3 0h2"
          class="stroke-primary-content"
          stroke-width="2"
        />
      </svg>
      runling
    </a>
    <div class="ml-auto flex items-center gap-3">
      <ThemePicker />
      <span
        class="hidden items-center gap-2 text-xs text-base-content/60 sm:flex"
      >
        <span
          class="status"
          class:status-success={listConnected}
          class:status-warning={!listConnected}
        ></span>
        {listConnected ? "Connected" : "Reconnecting"}
      </span>
      <button
        class="btn btn-primary btn-sm"
        disabled={!data.webhooks.length}
        onclick={() =>
          (composer =
            data.webhooks.find((hook) => hook.name === filter) ??
            data.webhooks[0] ??
            null)}
      >
        + New run
      </button>
    </div>
  </header>
  {#if notice}
    <div class="alert alert-info rounded-none" role="status">
      <span class="wrap-anywhere">{notice}</span>
      <button
        class="btn btn-ghost btn-xs"
        onclick={() => (notice = "")}
        aria-label="Dismiss notice">×</button
      >
    </div>
  {/if}
  <div
    class={[
      "grid min-h-0 flex-1",
      sidebarsExpanded
        ? "md:grid-cols-[min(var(--runs-width),40%)_minmax(0,1fr)] lg:grid-cols-[min(var(--webhook-width),25%)_min(var(--runs-width),30%)_minmax(0,1fr)]"
        : "grid-cols-1",
    ]}
    style:--webhook-width={webhookWidth === undefined ? "14rem" : `${webhookWidth}px`}
    style:--runs-width={runsWidth === undefined ? "17rem" : `${runsWidth}px`}
  >
    <aside
      class="relative min-w-0 flex flex-col border-b border-base-300 bg-base-200 p-3 md:col-span-2 lg:col-span-1 lg:overflow-y-auto lg:border-r lg:border-b-0 [&[hidden]]:hidden"
      id="webhook-sidebar"
      hidden={!sidebarsExpanded}
    >
      <div class="menu-title flex items-center justify-between px-3">
        <h2>Webhooks</h2>
        <span class="badge badge-ghost badge-sm">{data.webhooks.length}</span>
      </div>
      <ul class="menu menu-sm w-full gap-1">
        <li>
          <button class:menu-active={!filter} onclick={() => (filter = "")}>
            All runs <span class="badge badge-ghost badge-sm ml-auto"
              >{runs.length}</span
            >
          </button>
        </li>
      </ul>
      <ul
        class="menu menu-sm w-full gap-1 py-1 max-md:flex-row max-md:flex-wrap"
      >
        {#each data.webhooks as webhook}
          <li class="min-w-0 max-md:flex-1">
            <div class="flex items-center gap-0.5 p-0 hover:bg-transparent">
              <button
                class="min-w-0 flex-1 rounded-field px-2 py-1.5 text-left hover:bg-base-300"
                class:menu-active={filter === webhook.name}
                onclick={() => (filter = webhook.name)}
              >
                <span class="min-w-0"
                  ><strong class="block truncate">{webhook.name}</strong>
                  <span
                    class="block truncate text-xs text-base-content/60"
                    title={webhook.workflow}>{webhook.workflow}</span
                  >
                </span>
              </button>
              <div class="flex shrink-0 items-center">
                <button
                  class="btn btn-ghost btn-square btn-xs text-base-content/60"
                  onclick={() => (hookInfo = webhook)}
                  title={`Information about ${webhook.name}`}
                  aria-label={`Information about ${webhook.name}`}
                  ><span class="icon-[lucide--info] size-3.5" aria-hidden="true"
                  ></span></button
                >
                <button
                  class="btn btn-ghost btn-square btn-xs text-base-content/60"
                  onclick={() => copyUrl(webhook)}
                  aria-label={`Copy URL for ${webhook.name}`}
                  title={copied === webhook.name
                    ? "URL copied"
                    : "Copy webhook URL"}
                >
                  <span
                    class={copied === webhook.name
                      ? "icon-[lucide--check] size-3.5 text-success"
                      : "icon-[lucide--copy] size-3.5"}
                    aria-hidden="true"
                  ></span>
                </button>
                <button
                  class="btn btn-ghost btn-square btn-xs text-base-content/60"
                  onclick={() => (composer = webhook)}
                  title={`Run ${webhook.name}`}
                  aria-label={`Run ${webhook.name}`}
                  ><span class="icon-[lucide--play] size-3.5" aria-hidden="true"
                  ></span></button
                >
              </div>
            </div>
          </li>
        {:else}
          <li class="p-3 text-sm text-base-content/60">
            No webhooks configured. Add one to runling.config.ts. Changes load
            automatically.
          </li>
        {/each}
      </ul>
      <ColumnResizer bind:width={webhookWidth} storageKey="runling-width-webhooks" label="Webhooks column width" minWidth={160} class="hidden lg:block" />
    </aside>
    <section
      class="relative flex min-h-0 min-w-0 flex-col border-b border-base-300 bg-base-100 md:border-r md:border-b-0 [&[hidden]]:hidden"
      id="runs-sidebar"
      hidden={!sidebarsExpanded}
      aria-label="Workflow runs"
    >
      <div class="flex shrink-0 items-center justify-between gap-2 px-3 py-3">
        <h2 class="truncate text-sm font-semibold">
          {filter || "Recent runs"}
        </h2>
        <span class="badge badge-ghost badge-sm">{visibleRuns.length}</span>
      </div>
      <div
        class="flex min-h-0 flex-1 overflow-x-auto md:block lg:overflow-y-auto"
      >
        <ul class="list max-md:flex max-md:flex-row">
          {#each visibleRuns as run (run.id)}
            <li class="max-md:w-64 max-md:shrink-0">
              <button
                class={[
                  "w-full border-l-2 px-3 py-2 text-left transition-colors hover:bg-base-200 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary",
                  selected === run.id
                    ? "border-l-primary bg-primary/10"
                    : "border-l-transparent",
                ]}
                onclick={() => selectRun(run.id)}
                aria-pressed={selected === run.id}
                title={`Run ${run.id} · ${new Date(run.startedAt).toLocaleString()}`}
              >
                <span class="mb-1 flex items-center gap-2">
                  {#if run.status === "running" && !run.activity?.waiting}
                    <span class="inline-flex size-4 shrink-0 items-center justify-center text-info" role="img" aria-label="Running" title="Running">
                      <ActivityIndicator />
                    </span>
                  {:else}
                    <StatusBadge status={run.status} waiting={run.activity?.waiting} />
                  {/if}
                  <span class="min-w-0 flex-1 truncate text-sm font-medium"
                    >{run.workflow}</span
                  >
                  <time
                    class="shrink-0 text-xs text-base-content/50 tabular-nums"
                    datetime={new Date(run.startedAt).toISOString()}
                  >
                    {new Date(run.startedAt).toLocaleTimeString([], {
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </time>
                </span>
                {#if run.status === "running"}
                  <RunActivityPreview activity={run.activity} />
                {/if}
                <span
                  class="flex items-center justify-between gap-2 text-xs text-base-content/60"
                >
                  <Usage usage={run.usage} compact />
                  <span class="shrink-0"
                    >{run.durationMs !== undefined
                      ? duration(run.durationMs)
                      : "In progress"}</span
                  >
                </span>
              </button>
            </li>
          {:else}
            <li class="p-4 text-sm text-base-content/60">
              No runs yet. Start a run or send a webhook to see its progress
              here.
            </li>
          {/each}
        </ul>
      </div>
      <div
        class="hidden shrink-0 border-t border-base-300 px-4 py-3 text-xs text-base-content/50 md:block"
      >
        Latest 100 runs
      </div>
      <ColumnResizer bind:width={runsWidth} storageKey="runling-width-runs" label="Runs column width" minWidth={180} class="hidden md:block" />
    </section>
    <main class="min-w-0 bg-base-100 lg:overflow-y-auto">
      {#if detail}
        {#key detail.id}<RunInspector run={detail} {connection} />{/key}
      {:else if selected}
        <div class="hero min-h-96 p-8">
          <div class="hero-content flex-col text-center">
            {#if !detailError}<span
                class="loading loading-spinner loading-lg text-primary"
              ></span>{/if}
            <h1 class="text-2xl font-semibold">
              {detailError ? "Run unavailable" : "Loading run"}
            </h1>
            <p class="text-base-content/60">{detailError || connection}</p>
            {#if detailError}<button
                class="btn btn-sm"
                onclick={() => selectRun(selected)}>Retry</button
              >{/if}
          </div>
        </div>
      {:else}
        <div class="hero min-h-96 p-8">
          <div class="hero-content flex-col text-center">
            <h1 class="text-3xl font-semibold tracking-tight">
              No run selected
            </h1>
            <p class="max-w-sm text-base-content/60">
              Select a run to view its steps and results, or start a new run.
            </p>
            <button
              class="btn btn-primary"
              disabled={!data.webhooks.length}
              onclick={() => (composer = data.webhooks[0] ?? null)}
              >Start a run</button
            >
          </div>
        </div>
      {/if}
    </main>
  </div>
</div>
{#if hookInfo}<WebhookInfoModal
    webhook={hookInfo}
    onclose={() => (hookInfo = null)}
  />{/if}
{#if composer}<RunComposer
    webhook={composer}
    onclose={() => (composer = null)}
    onstarted={(id) => {
      filter = "";
      selectRun(id);
    }}
  />{/if}

{#if !listConnected}<ConnectionOverlay />{/if}
