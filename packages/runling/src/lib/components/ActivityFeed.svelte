<script lang="ts">
  import { tick } from "svelte";
  import type { RunDetail } from "$lib/runs.ts";
  import { activityFeed, feedTime, type FeedEntry } from "$lib/activity-feed.ts";
  let { run }: { run: RunDetail } = $props();
  const entries = $derived(activityFeed(run.events, run.status, run.durationMs));
  let limit = $state(500);
  const visible = $derived(entries.slice(-limit));
  let following = $state(true);
  let seen = $state(0);
  let viewport: HTMLDivElement;
  let previousTop = 0;
  const unread = $derived(Math.max(0, entries.length - seen));
  const toneClass = (tone: FeedEntry["tone"]) => tone === "waiting" ? "text-base-content/45"
    : tone === "error" ? "text-error" : tone === "success" ? "text-success" : "text-base-content/80";

  function jump() {
    following = true;
    viewport.scrollTop = viewport.scrollHeight;
    previousTop = viewport.scrollTop;
    seen = entries.length;
  }

  function scroll() {
    if (viewport.scrollTop < previousTop - 1) following = false;
    if (viewport.scrollHeight - viewport.clientHeight - viewport.scrollTop <= 8) {
      following = true;
      seen = entries.length;
    }
    previousTop = viewport.scrollTop;
  }

  // Follow new text and layout changes only while the reader is at the bottom.
  // No timer and no smooth-scroll queue: a busy run stays attached to the bottom.
  function follow(node: HTMLDivElement, _count: number) {
    let alive = true;
    const refresh = async () => { await tick(); if (alive && following) jump(); };
    const observer = new ResizeObserver(refresh);
    observer.observe(node);
    if (node.firstElementChild) observer.observe(node.firstElementChild);
    void refresh();
    return { update() { void refresh(); }, destroy() { alive = false; observer.disconnect(); } };
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
</script>

<section aria-label="Run activity">
  <div bind:this={viewport} use:follow={entries.length} onscroll={scroll}
    class="h-[min(60vh,44rem)] min-h-64 overflow-auto overscroll-contain [overflow-anchor:none] [scrollbar-gutter:stable]"
    role="region" aria-label="Activity entries">
    <div class="px-2 py-3">
      {#if entries.length > limit}
        <button class="btn btn-ghost btn-sm mx-4 my-2" onclick={earlier}>Show earlier activity ({entries.length - limit})</button>
      {/if}
      <ol class="m-0 list-none space-y-1 p-0 font-mono text-xs leading-relaxed">
        {#each visible as entry (entry.id)}
          <li class="whitespace-pre-wrap break-words" data-feed-entry={entry.id}>
            <span class="text-base-content/35" title="Elapsed time">{feedTime(entry.timestamp)}</span>{" "}<span style:color={entry.color} title={entry.reference}>[{entry.task}]</span>{" "}<span class={toneClass(entry.tone)}>{entry.message}{entry.detail ? `\n${entry.detail}` : ""}</span>
          </li>
        {:else}
          <li class="px-5 py-14 text-center text-sm text-base-content/50">Waiting for activity…</li>
        {/each}
      </ol>
    </div>
  </div>
  {#if !following}
    <div class="flex justify-center border-t border-base-300 bg-base-200/50 p-2">
      <button class="btn btn-sm btn-ghost gap-2" onclick={jump}><span aria-hidden="true">↓</span>{unread ? `${unread} new ${unread === 1 ? "event" : "events"}` : "Follow latest"}</button>
    </div>
  {/if}
</section>
