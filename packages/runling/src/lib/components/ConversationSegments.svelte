<script lang="ts">
  import type { Activity } from "$lib/timeline.ts";
  import { duration } from "$lib/runs.ts";
  import type { TimeWindow } from "$lib/timeline-layout.ts";

  let { segments, start, end, elapsed, view, width }: {
    segments: Activity[];
    start: number;
    end: number;
    elapsed: number;
    view: TimeWindow;
    width: number;
  } = $props();

  let visibleStart = $derived(Math.max(start, view.start));
  let visibleEnd = $derived(Math.min(end, view.start + view.span));
  let span = $derived(Math.max(1, visibleEnd - visibleStart));
</script>

{#each segments as segment (segment.id)}
  {@const end = segment.startedAt + (segment.durationMs ?? Math.max(0, elapsed - segment.startedAt))}
  {@const from = Math.max(segment.startedAt, visibleStart)}
  {@const to = Math.min(end, visibleEnd)}
  {@const label = segment.kind === "input" ? "Waiting" : "Working"}
  {#if to > from}
    <span
      class={[
        "absolute inset-y-0 flex items-center justify-center overflow-hidden text-xs whitespace-nowrap",
        segment.kind === "input" ? "bg-warning text-warning-content" : "bg-secondary text-secondary-content",
      ]}
      style:left={`${((from - visibleStart) / span) * 100}%`}
      style:width={`${((to - from) / span) * 100}%`}
      title={`${label} · ${duration(end - segment.startedAt)}`}
    >{#if ((to - from) / view.span) * width >= 64}{label}{/if}</span>
  {/if}
{/each}
