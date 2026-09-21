<script lang="ts">
  import type { Activity } from "$lib/timeline.ts";
  import { zoomWindow, type TimeWindow } from "$lib/timeline-layout.ts";
  import { overviewSpan, moveOverview } from "$lib/minimap.ts";
  import { duration } from "$lib/runs.ts";
  import { middleDrag } from "$lib/middle-drag.ts";
  let {
    rows,
    view,
    vertical,
    elapsed,
    selected,
    onchange,
    onfit,
    onzoomstart,
  }: {
    rows: { node: Activity; top: number; height: number }[];
    view: TimeWindow;
    vertical: { start: number; span: number; total: number };
    elapsed: number;
    selected: string;
    onchange: (view: TimeWindow, top: number) => void;
    onfit: () => void;
    onzoomstart: (anchor: number) => (dx: number, dy: number) => void;
  } = $props();
  let middleDragging = $state(false);
  function attachMiddleDrag(element: HTMLElement) {
    return middleDrag(
      element,
      (event) => {
        const rect = element.getBoundingClientRect();
        if (!rect.width || !rect.height) return;
        const initial = { ...view };
        const time = ((event.clientX - rect.left) / rect.width) * total;
        const anchor = (time - initial.start) / initial.span;
        const zoomBoth = onzoomstart(anchor);
        middleDragging = true;
        return (dx, dy) => zoomBoth(dx, dy);
      },
      () => {
        middleDragging = false;
      },
    );
  }
  let gesture = $state<{
    id: number;
    rect: DOMRect;
    total: number;
    axis: typeof vertical;
    view: TimeWindow;
    x: number;
    y: number;
  }>();
  let total = $derived(gesture?.total ?? overviewSpan(elapsed));
  let axis = $derived(gesture?.axis ?? vertical);
  let visible = $derived(Math.min(axis.span, axis.total));
  let top = $derived(
    Math.min(vertical.start, Math.max(0, axis.total - visible)),
  );

  function down(event: PointerEvent) {
    if (event.pointerType === "mouse" && event.button === 1) return;
    if ((event.button !== 0 && event.button !== 1) || gesture) return;
    event.preventDefault();
    const element = event.currentTarget as HTMLElement;
    const rect = element.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const x = ((event.clientX - rect.left) / rect.width) * total;
    const y = ((event.clientY - rect.top) / rect.height) * axis.total;
    const inside =
      x >= view.start &&
      x <= view.start + view.span &&
      y >= top &&
      y <= top + visible;
    gesture = {
      id: event.pointerId,
      rect,
      total,
      axis: { ...axis },
      view: { ...view },
      x: inside || event.button === 1 ? x - view.start : view.span / 2,
      y: inside || event.button === 1 ? y - top : visible / 2,
    };
    element.setPointerCapture(event.pointerId);
    element.focus();
    move(event);
  }
  function move(event: PointerEvent) {
    if (!gesture || gesture.id !== event.pointerId) return;
    const g = gesture;
    const x = ((event.clientX - g.rect.left) / g.rect.width) * g.total;
    const y = ((event.clientY - g.rect.top) / g.rect.height) * g.axis.total;
    onchange(
      moveOverview(g.view, g.total, x, g.x),
      moveOverview(
        { start: g.axis.start, span: Math.min(g.axis.span, g.axis.total) },
        g.axis.total,
        y,
        g.y,
      ).start,
    );
  }
  function end(event: PointerEvent) {
    if (gesture?.id !== event.pointerId) return;
    gesture = undefined;
    const element = event.currentTarget as HTMLElement;
    if (element.hasPointerCapture(event.pointerId))
      element.releasePointerCapture(event.pointerId);
  }
  function preventMiddleDefault(event: MouseEvent) {
    if (event.button === 1) event.preventDefault();
  }
  function key(event: KeyboardEvent) {
    if (event.key === "ArrowLeft" || event.key === "ArrowRight")
      onchange(
        moveOverview(
          view,
          total,
          view.start + (event.key === "ArrowLeft" ? -1 : 1) * view.span * 0.15,
          0,
        ),
        top,
      );
    else if (event.key === "ArrowUp" || event.key === "ArrowDown")
      onchange(
        view,
        moveOverview(
          { start: top, span: visible },
          axis.total,
          top + (event.key === "ArrowUp" ? -1 : 1) * visible * 0.2,
          0,
        ).start,
      );
    else if (event.key === "Home") onchange({ ...view, start: 0 }, 0);
    else if (event.key === "End")
      onchange(
        { ...view, start: Math.max(0, total - view.span) },
        axis.total - visible,
      );
    else if (event.key === "+" || event.key === "=")
      onchange(zoomWindow(view, 0.7), top);
    else if (event.key === "-") onchange(zoomWindow(view, 1 / 0.7), top);
    else if (event.key.toLowerCase() === "f") onfit();
    else return;
    event.preventDefault();
    event.stopPropagation();
  }
</script>

<div class="min-w-0 py-2 pr-0.5">
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Two-dimensional overview supports pointer and keyboard navigation.) -->
  <div
    class={[
      "relative bg-base-100 border border-base-300 rounded-field overflow-hidden touch-none focus-visible:outline-2 focus-visible:outline-primary focus-visible:outline-offset-2",
      gesture || middleDragging ? "cursor-grabbing" : "cursor-grab",
    ]}
    style:height={`${Math.min(160, Math.max(80, rows.length * 3))}px`}
    {@attach attachMiddleDrag}
    role="region"
    tabindex="0"
    aria-label={`Timeline minimap, ${duration(view.start)} to ${duration(view.start + view.span)}. Arrow keys pan in both directions; plus and minus zoom; F fits time.`}
    onpointerdown={down}
    onpointermove={move}
    onpointerup={end}
    onpointercancel={end}
    onlostpointercapture={end}
    onmousedown={preventMiddleDefault}
    onauxclick={preventMiddleDefault}
    onkeydown={key}
  >
    <svg
      class="absolute w-full h-full pointer-events-none"
      viewBox={`0 0 1000 ${axis.total}`}
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      {#each rows as { node, top, height } (node.id)}
        <rect
          class={[
            node.status === "failed" || node.status === "blocked"
              ? "fill-error light:fill-rose-300"
              : node.status === "interrupted"
                ? "fill-neutral light:fill-slate-300"
                : node.kind === "agent"
                  ? "fill-secondary light:fill-violet-300"
                  : node.kind === "command"
                    ? "fill-accent light:fill-teal-300"
                    : node.kind === "input"
                      ? "fill-warning light:fill-amber-300"
                      : "fill-primary light:fill-blue-300",
            node.id === selected &&
              "stroke-base-content stroke-1 [vector-effect:non-scaling-stroke]",
          ]}
          x={(node.startedAt / total) * 1000}
          y={top + height * 0.2}
          width={Math.max(
            1,
            ((node.durationMs ?? Math.max(0, elapsed - node.startedAt)) /
              total) *
              1000,
          )}
          height={height * 0.6}
          data-kind={node.kind}
        />
        {#each node.segments ?? [] as segment}
          <rect
            class={segment.kind === "input" ? "fill-warning" : "fill-secondary"}
            x={(segment.startedAt / total) * 1000}
            y={top + height * 0.2}
            width={Math.max(1, ((segment.durationMs ?? Math.max(0, elapsed - segment.startedAt)) / total) * 1000)}
            height={height * 0.6}
          />
        {/each}
      {/each}
    </svg>
    <span
      class="absolute pointer-events-none border-2 border-primary bg-primary/5 light:border-slate-400 light:bg-slate-400/5 min-w-0.5 min-h-0.5 box-border shadow-[0_0_0_100vmax] shadow-base-300/60"
      style:left={`${(view.start / total) * 100}%`}
      style:width={`${(view.span / total) * 100}%`}
      style:top={`${(top / axis.total) * 100}%`}
      style:height={`${(visible / axis.total) * 100}%`}
    ></span>
    <span
      class="absolute pointer-events-none top-0 bottom-0 w-0.5 bg-error"
      style:left={`${(elapsed / total) * 100}%`}
    ></span>
  </div>
  <div
    class="flex justify-between items-center text-base-content/60 text-xs mt-1 tabular-nums"
  >
    <span>0</span>
    <span>{rows.length ? `Rows ${Math.min(rows.length, Math.floor(top / axis.total * rows.length) + 1)}–${Math.min(rows.length, Math.ceil((top + visible) / axis.total * rows.length))} of ${rows.length}` : "No rows"}</span>
    <span>{duration(total)}</span>
  </div>
</div>
