<script lang="ts">
  import { tick } from "svelte";
  import { ansiTokens } from "$lib/ansi.ts";
  import { logTime, nextLogFollow, type LogRow } from "$lib/run-log.ts";
  import AnsiText from "./AnsiText.svelte";

  let { rows, running }: { rows: LogRow[]; running: boolean } = $props();
  let viewport: HTMLDivElement;
  let limit = $state(500);
  let wrap = $state(true);
  let following = $state(true);
  let seen = $state(0);
  let copyState = $state<"idle" | "copied" | "failed">("idle");
  let expandable = $state.raw(new Set<number>());
  let expanded = $state.raw(new Set<number>());
  let previousTop = 0;
  const measuredLines = new Map<Element, number>();
  let lineObserver: ResizeObserver | undefined;
  let visible = $derived(rows.slice(-limit));
  let unread = $derived(Math.max(0, rows.length - seen));

  const marker = (level: LogRow["event"]["level"]) =>
    level === "success" ? "✓" : level === "error" ? "✕" : level === "debug" ? "·" : "•";
  const markerClass = (level: LogRow["event"]["level"]) =>
    level === "success" ? "text-success" : level === "error" ? "text-error" : level === "debug" ? "text-base-content/35" : "text-info";

  function measureLine(node: HTMLElement, id: number) {
    lineObserver ??= new ResizeObserver((entries) => {
      const next = new Set(expandable);
      let changed = false;
      for (const entry of entries) {
        const rowId = measuredLines.get(entry.target);
        if (rowId === undefined) continue;
        const long = (entry.target as HTMLElement).scrollHeight > 80;
        if (long !== next.has(rowId)) {
          changed = true;
          if (long) next.add(rowId);
          else next.delete(rowId);
        }
      }
      if (changed) expandable = next;
    });
    measuredLines.set(node, id);
    lineObserver.observe(node);
    return {
      destroy() {
        lineObserver?.unobserve(node);
        measuredLines.delete(node);
      },
    };
  }

  function toggleLine(id: number) {
    const next = new Set(expanded);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    expanded = next;
    following = false;
    previousTop = viewport.scrollTop;
  }

  function jump() {
    following = true;
    viewport.scrollTop = viewport.scrollHeight;
    previousTop = viewport.scrollTop;
    seen = rows.length;
  }

  function scroll() {
    following = nextLogFollow(following, previousTop, viewport);
    if (following) seen = rows.length;
    previousTop = viewport.scrollTop;
  }

  // Follow new records and changes in line height without queuing smooth scrolls.
  function follow(node: HTMLDivElement, _count: number) {
    let alive = true;
    const refresh = async () => {
      await tick();
      if (alive && following) jump();
    };
    const observer = new ResizeObserver(refresh);
    observer.observe(node);
    if (node.firstElementChild) observer.observe(node.firstElementChild);
    void refresh();
    return {
      update() { void refresh(); },
      destroy() { alive = false; observer.disconnect(); },
    };
  }

  async function earlier() {
    following = false;
    const height = viewport.scrollHeight;
    const top = viewport.scrollTop;
    limit += 500;
    await tick();
    viewport.scrollTop = top + viewport.scrollHeight - height;
    previousTop = viewport.scrollTop;
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(
        rows.map(({ event }) => ansiTokens(event.message).map((token) => token.text).join("")).join("\n"),
      );
      copyState = "copied";
    } catch {
      copyState = "failed";
    }
  }
</script>

{#snippet message(row: LogRow)}
  <span
    use:measureLine={row.id}
    class={["block min-w-0 text-base-content/90", wrap ? "whitespace-pre-wrap wrap-anywhere" : "whitespace-pre"]}
  ><AnsiText text={row.event.message} /></span>
{/snippet}

<section class="flex min-h-0 flex-1 flex-col" aria-label="Run log">
  <div class="flex shrink-0 items-center justify-between gap-3 border-b border-base-300 bg-base-200/35 px-4 py-1.5 sm:px-6">
    <span class="text-xs text-base-content/50 tabular-nums">{rows.length} {rows.length === 1 ? "line" : "lines"}</span>
    <div class="flex items-center gap-1">
      <button class="btn btn-ghost btn-xs gap-1.5" aria-pressed={wrap} onclick={() => (wrap = !wrap)}>
        <span class="icon-[lucide--wrap-text] size-3.5" aria-hidden="true"></span>Wrap
      </button>
      <button class="btn btn-ghost btn-xs gap-1.5" onclick={copy}>
        <span class={copyState === "copied" ? "icon-[lucide--check] size-3.5" : "icon-[lucide--copy] size-3.5"} aria-hidden="true"></span>
        {copyState === "copied" ? "Copied" : "Copy log"}
      </button>
    </div>
  </div>
  <div
    bind:this={viewport}
    use:follow={rows.length}
    onscroll={scroll}
    class="min-h-0 flex-1 overflow-auto overscroll-contain [overflow-anchor:none] [scrollbar-gutter:stable]"
    role="region"
    aria-label="Run log lines"
  >
    <div class="min-h-full px-2 py-3 sm:px-4">
      {#if rows.length > limit}
        <button class="btn btn-ghost btn-sm mb-3 ml-2" onclick={earlier}>Show earlier logs ({rows.length - limit})</button>
      {/if}
      <ol class="m-0 list-none p-0 font-mono text-[13px] leading-5">
        {#each visible as row (row.id)}
          <li class="grid grid-cols-[4.5rem_1rem_minmax(0,1fr)] gap-x-2 rounded px-2 py-0.5 hover:bg-base-200/55 sm:grid-cols-[5rem_1rem_minmax(0,1fr)]" data-log-row={row.id}>
            <time class="select-none text-base-content/40 tabular-nums" title="Elapsed run time">{logTime(row.event.timestamp)}</time>
            <span class={markerClass(row.event.level)} title={row.event.level} aria-label={row.event.level}>{marker(row.event.level)}</span>
            {#if expandable.has(row.id)}
              <button
                class="block min-w-0 w-full cursor-pointer font-mono text-left text-[13px] leading-5 focus-visible:rounded focus-visible:outline-2 focus-visible:outline-primary"
                class:cursor-zoom-out={expanded.has(row.id)}
                aria-expanded={expanded.has(row.id)}
                onclick={() => toggleLine(row.id)}
                style:padding-left={`${Math.min(Math.max(row.event.depth, 0), 6) * 0.75}rem`}
              >
                <span class={expanded.has(row.id) ? "block" : "block max-h-15 overflow-y-clip overflow-x-visible"}>
                  {@render message(row)}
                </span>
                {#if expanded.has(row.id)}
                  <span class="mt-1 block text-right text-[11px] text-primary">Show less ↑</span>
                {:else}
                  <span class="block text-right text-[11px] text-primary">Show more ↓</span>
                {/if}
              </button>
            {:else}
              <span class="min-w-0 max-h-20 overflow-y-clip overflow-x-visible" style:padding-left={`${Math.min(Math.max(row.event.depth, 0), 6) * 0.75}rem`}>
                {@render message(row)}
              </span>
            {/if}
          </li>
        {:else}
          <li class="px-4 py-12 text-center font-sans text-sm text-base-content/50">
            {running ? "Waiting for log lines…" : "No log lines were recorded."}
          </li>
        {/each}
      </ol>
    </div>
  </div>
  {#if !following}
    <div class="flex shrink-0 justify-center border-t border-base-300 bg-base-200/50 p-2">
      <button class="btn btn-sm btn-ghost gap-2" onclick={jump}><span aria-hidden="true">↓</span>{unread ? `${unread} new ${unread === 1 ? "line" : "lines"}` : "Follow latest"}</button>
    </div>
  {/if}
  <p class="sr-only" role="status">{copyState === "copied" ? "Log copied to clipboard." : ""}</p>
  {#if copyState === "failed"}<p class="px-4 py-2 text-xs text-error" role="alert">Could not copy the log. Select and copy the text manually.</p>{/if}
</section>
