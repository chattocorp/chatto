<script lang="ts">
  import { onMount, tick } from "svelte";
  import ConversationSegments from "./ConversationSegments.svelte";
  import ColumnResizer from "./ColumnResizer.svelte";
  import Usage from "./Usage.svelte";
  import AnsiText from "./AnsiText.svelte";
  import TaskMessageInspector from "./TaskMessageInspector.svelte";
  import TaskMessageMarkers from "./TaskMessageMarkers.svelte";
  import TimelineMinimap from "./TimelineMinimap.svelte";
  import { middleDrag } from "$lib/middle-drag.ts";
  import { duration } from "$lib/runs.ts";
  import { findActivity, isActivityActive, activityStatus, type Activity, type TaskMessage } from "$lib/timeline.ts";
  import ActivityIndicator from "./ActivityIndicator.svelte";
  import {
    barPosition,
    fitWindow,
    timelineEnd,
    clampWindow,
    flattenActivities,
    panWindow,
    timelineTicks,
    tickLabel,
    zoomWindow,
    dragZoomWindow,
    dragRowHeight,
    type TimeWindow,
  } from "$lib/timeline-layout.ts";

  let {
    nodes,
    selected,
    onselect,
    elapsed,
    running,
  }: {
    nodes: Activity[];
    selected: string;
    onselect: (id: string) => void;
    elapsed: number;
    running: boolean;
  } = $props();
  let selectedMessageId = $state<string>();
  let activityWidth = $state<number>();
  let overview = $state(true);
  function toggleOverview() {
    overview = !overview;
    try {
      localStorage.setItem("runling-timeline-overview", String(overview));
    } catch { /* Storage can be unavailable. */ }
  }
  let board: HTMLDivElement;
  let ruler: HTMLDivElement;
  let scrollTop = $state(0);
  let visibleHeight = $state(1);
  let rowHeight = $state(48);
  function beginMiddleZoom(anchor: number, y = visibleHeight / 2) {
    const initialManual = manual;
    const initial = { ...view };
    const initialHeight = rowHeight;
    const rowAnchor = (board.scrollTop + y) / initialHeight;
    return async (dx: number, dy: number) => {
      manual = dx === 0 ? initialManual : dragZoomWindow(initial, -dx, anchor);
      const height = dragRowHeight(initialHeight, dy);
      rowHeight = height;
      await tick();
      if (rowHeight === height)
        board.scrollTop = Math.max(0, rowAnchor * height - y);
    };
  }
  let panel: HTMLElement;
  let expandButton: HTMLButtonElement;
  let manual = $state<TimeWindow | null>(null);
  let collapsed = $state<Set<string>>(new Set());
  let plotWidth = $state(500);
  let expanded = $state(false);
  let dragging = $state(false);
  let extent = $derived(timelineEnd(nodes, elapsed));
  let view = $derived(
    clampWindow(!manual || manual.span >= elapsed ? fitWindow(extent) : manual, fitWindow(extent).span),
  );
  let rows = $derived(flattenActivities(nodes, collapsed));
  let message = $derived(rows.flatMap(row => row.node.messages ?? []).find(item => item.id === selectedMessageId));
  let contentHeight = $derived(Math.max(1, rows.length * rowHeight));
  let miniRows = $derived(
    rows.map(({ node }, index) => ({
      node,
      top: index * rowHeight,
      height: rowHeight,
    })),
  );
  let ticks = $derived(timelineTicks(view, plotWidth));
  let cursor = $derived(((extent - view.start) / view.span) * 100);
  let zoom = $derived(fitWindow(extent).span / view.span);
  let inspected = $derived(findActivity(nodes, selected));

  function toggle(id: string) {
    const next = new Set(collapsed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    collapsed = next;
  }
  function fit() {
    manual = null;
  }
  async function toggleExpanded() {
    expanded = !expanded;
    await tick();
    if (expanded) board.focus();
    else expandButton.focus();
  }
  function globalKey(event: KeyboardEvent) {
    // A message dialog owns Escape and focus while it is open.
    if (!expanded || (event.target instanceof Element && event.target.closest("dialog"))) return;
    if (event.key === "Escape") {
      event.preventDefault();
      void toggleExpanded();
    }
    if (event.key === "Tab") {
      const controls = [
        ...panel.querySelectorAll<HTMLElement>('button, [tabindex="0"]'),
      ].filter((element) => !element.closest("[inert]"));
      const first = controls[0];
      const last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    }
  }
  function zoomBy(factor: number, anchor = 0.5) {
    manual = zoomWindow(view, factor, anchor);
  }
  function keyboard(event: KeyboardEvent) {
    if (event.target !== board) return;
    if (event.key === "+" || event.key === "=") zoomBy(0.7);
    else if (event.key === "-") zoomBy(1 / 0.7);
    else if (event.key === "ArrowLeft")
      manual = panWindow(view, -view.span * 0.15);
    else if (event.key === "ArrowRight")
      manual = panWindow(view, view.span * 0.15);
    else if (event.key === "Home" || event.key.toLowerCase() === "f") fit();
    else return;
    event.preventDefault();
  }

  onMount(() => {
    try {
      overview = localStorage.getItem("runling-timeline-overview") !== "false";
    } catch { /* Use the default when storage is unavailable. */ }
    const observer = new ResizeObserver(() => {
      plotWidth = Math.max(1, ruler.clientWidth);
      visibleHeight = Math.max(1, board.clientHeight - ruler.offsetHeight);
      scrollTop = board.scrollTop;
    });
    observer.observe(ruler);
    observer.observe(board);
    const wheel = (event: WheelEvent) => {
      if (
        !(event.target instanceof Element) ||
        !event.target.closest("[data-timeline-plot]")
      )
        return;
      event.preventDefault();
      const unit =
        event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? plotWidth : 1;
      if (
        event.shiftKey ||
        (Math.abs(event.deltaX) > Math.abs(event.deltaY) && !event.ctrlKey)
      ) {
        manual = panWindow(
          view,
          (((event.deltaX || event.deltaY) * unit) / plotWidth) * view.span,
        );
      } else {
        const anchor =
          (event.clientX - ruler.getBoundingClientRect().left) / plotWidth;
        zoomBy(
          Math.exp(Math.max(-1, Math.min(1, event.deltaY * unit * 0.004))),
          anchor,
        );
      }
    };
    const pointers = new Map<number, { x: number; y: number }>();
    let gesture: {
      view: TimeWindow;
      x: number;
      y: number;
      distance: number;
      scroll: number;
    } | null = null;
    let suppressUntil = 0;
    const center = () => {
      const values = [...pointers.values()];
      const first = values[0]!;
      const second = values[1];
      return {
        x: second ? (first.x + second.x) / 2 : first.x,
        y: second ? (first.y + second.y) / 2 : first.y,
        distance: second
          ? Math.hypot(first.x - second.x, first.y - second.y)
          : 0,
      };
    };
    const begin = () => {
      gesture = { ...center(), view: { ...view }, scroll: board.scrollTop };
    };
    const down = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest("[data-message-marker]")) return;
      if (event.pointerType === "mouse" && event.button === 1) return;
      if (
        (event.button !== 0 && event.button !== 1) ||
        !(event.target instanceof Element) ||
        (event.button !== 1 && !event.target.closest("[data-timeline-plot]"))
      )
        return;
      if (event.button === 1) event.preventDefault();
      pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
      begin();
    };
    const move = (event: PointerEvent) => {
      if (!pointers.has(event.pointerId) || !gesture) return;
      pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
      const current = center();
      const dx = current.x - gesture.x;
      const dy = current.y - gesture.y;
      if (!dragging && pointers.size === 1 && Math.hypot(dx, dy) < 4) return;
      dragging = true;
      board.setPointerCapture(event.pointerId);
      const anchor =
        (gesture.x - ruler.getBoundingClientRect().left) / plotWidth;
      const zoomed =
        gesture.distance > 0 && current.distance > 0
          ? zoomWindow(
              gesture.view,
              gesture.distance / current.distance,
              anchor,
            )
          : gesture.view;
      manual = panWindow(zoomed, (-dx / plotWidth) * zoomed.span);
      board.scrollTop = gesture.scroll - dy;
    };
    const up = (event: PointerEvent) => {
      if (!pointers.delete(event.pointerId)) return;
      if (dragging) suppressUntil = performance.now() + 200;
      if (pointers.size) begin();
      else {
        gesture = null;
        dragging = false;
      }
    };
    const click = (event: MouseEvent) => {
      if (event.target instanceof Element && event.target.closest("[data-message-marker]")) return;
      if (event.detail && performance.now() < suppressUntil) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    const stopMiddleDrag = middleDrag(
      board,
      (event) => {
        const anchor =
          (event.clientX - ruler.getBoundingClientRect().left) / plotWidth;
        const y = Math.max(
          0,
          Math.min(
            visibleHeight,
            event.clientY -
              board.getBoundingClientRect().top -
              ruler.offsetHeight,
          ),
        );
        const zoomBoth = beginMiddleZoom(anchor, y);
        dragging = true;
        return (dx, dy) => {
          void zoomBoth(dx, dy);
        };
      },
      () => {
        dragging = false;
        suppressUntil = performance.now() + 200;
      },
    );
    board.addEventListener("wheel", wheel, { passive: false });
    board.addEventListener("pointerdown", down);
    board.addEventListener("click", click, true);
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    return () => {
      observer.disconnect();
      stopMiddleDrag();
      board.removeEventListener("wheel", wheel);
      board.removeEventListener("pointerdown", down);
      board.removeEventListener("click", click, true);
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
    };
  });
</script>

<svelte:window onkeydown={globalKey} />

<section
  class={[
    "border border-base-300 rounded-box bg-base-100 overflow-hidden [--default-activity-width:clamp(125px,29%,235px)] max-sm:[--default-activity-width:120px]",
    expanded &&
      "fixed inset-2 z-50 flex flex-col shadow-2xl ring-32 ring-black/50 sm:inset-5",
  ]}
  style:--activity-width={activityWidth === undefined ? "var(--default-activity-width)" : `min(${activityWidth}px, 65%)`}
  style:--row-height={`${rowHeight}px`}
  aria-label="Execution timeline"
  bind:this={panel}
>
  <div
    class="min-h-12 flex items-center justify-between gap-2.5 py-2 px-3 border-b border-b-base-300 bg-base-100 max-sm:p-2"
  >
    <div
      class="flex items-center gap-2 text-xs text-base-content/60 whitespace-nowrap"
    >
      <span
        class={[
          "status status-sm",
          running ? "status-primary" : "bg-base-content/40",
        ]}
      ></span>{manual ? "Explore" : running ? "Live fit" : "Full timeline"}<span
        class="text-base-content/60 pl-1.5 tabular-nums max-sm:hidden"
        >{zoom.toFixed(zoom < 10 ? 1 : 0)}×</span
      >
    </div>
    <div class="flex items-center flex-wrap gap-1.5">
      <button
        class="btn btn-xs btn-square"
        onclick={() => zoomBy(1 / 0.7)}
        aria-label="Zoom out"
        disabled={zoom <= 1}
        title="Zoom out (−)">−</button
      >
      <button
        class="btn btn-xs btn-square"
        onclick={() => zoomBy(0.7)}
        aria-label="Zoom in"
        title="Zoom in (+)">+</button
      >
      <button
        class="btn btn-xs"
        class:btn-active={!manual}
        onclick={fit}
        title="Fit full timeline (F)">Fit timeline</button
      >
      <button
        class="btn btn-xs"
        class:btn-active={overview}
        aria-pressed={overview}
        onclick={toggleOverview}
        title={overview ? "Hide overview" : "Show overview"}
      >Overview</button>
      <button
        class="btn btn-xs btn-square"
        bind:this={expandButton}
        onclick={toggleExpanded}
        aria-label={expanded ? "Exit expanded timeline" : "Expand timeline"}
        title={expanded ? "Exit expanded view (Escape)" : "Expand timeline"}
        >{expanded ? "↙" : "↗"}</button
      >
    </div>
  </div>
  <div class={["relative min-h-0", expanded && "flex-1"]}>
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (The scrollable chart is focusable for its documented pan and zoom keyboard controls.) -->
    <div
      class={[
        "overflow-y-auto overflow-x-hidden -outline-offset-2 [scrollbar-gutter:stable]",
        expanded ? "h-full" : "h-120",
        dragging && "cursor-grabbing",
      ]}
      bind:this={board}
      tabindex="0"
      role="region"
      aria-roledescription="Interactive timeline"
      aria-label="Timeline chart. Drag to pan, scroll to zoom. Arrow keys pan, plus and minus zoom, F fits the timeline."
      onkeydown={keyboard}
      onscroll={() => (scrollTop = board.scrollTop)}
    >
      <div
        class="relative grid grid-cols-[var(--activity-width)_minmax(0,1fr)] grid-rows-[35px] auto-rows-auto min-w-0"
      >
        <div
          class="sticky top-0 z-3 bg-base-200 border-b border-b-base-300 text-xs text-base-content/60 flex items-center justify-between py-0 px-3 border-r border-r-base-300"
        >
          Activity <span class="text-xs text-base-content/60"
            >{rows.length}</span
          >
        </div>
        <div
          class={[
            "sticky top-0 z-3 bg-base-200 border-b border-base-300 overflow-hidden touch-none select-none",
            dragging ? "cursor-grabbing" : "cursor-grab",
          ]}
          data-timeline-plot
          bind:this={ruler}
        >
          {#each ticks as tick (tick)}
            <span
              class="absolute top-0 h-full pt-2.5 pr-0 pb-0 pl-1.5 border-l border-l-base-300 whitespace-nowrap text-base-content/60 text-xs tabular-nums"
              style:left={`${((tick - view.start) / view.span) * 100}%`}
              >{tickLabel(
                tick,
                ticks.length > 1 ? ticks[1]! - ticks[0]! : view.span / 5,
              )}</span
            >
          {/each}
          {#if cursor >= 0 && cursor <= 100}
            <span
              class={[
                "absolute bottom-0 text-xs px-1 rounded-t-sm pointer-events-none",
                cursor > 95 ? "-translate-x-full" : cursor < 5 ? "translate-x-0" : "-translate-x-1/2",
                running
                  ? "bg-error text-error-content"
                  : "bg-neutral text-neutral-content",
              ]}
              style:left={`${cursor}%`}>{running ? "Now" : "End"}</span
            >
          {/if}
        </div>
        {#snippet branch(node: Activity, depth: number)}
          {@const end =
            node.startedAt +
            (node.durationMs ?? Math.max(0, elapsed - node.startedAt))}
          {@const bar = barPosition(node.startedAt, end, view)}
          <div
            class="col-span-2 grid grid-cols-subgrid h-(--row-height) min-h-0"
            data-timeline-row={node.id}
          >
            <div
              class={[
                "overflow-hidden flex items-center border-b border-r border-base-300 pr-1.5 min-w-0",
                selected === node.id ? "bg-primary/10" : "bg-base-200/50",
              ]}
              style:padding-left={`${8 + Math.min(depth, 6) * 13}px`}
            >
              {#if node.children.length}
                <button
                  class="btn btn-ghost btn-xs w-5 px-0 shrink-0"
                  aria-label={`${collapsed.has(node.id) ? "Expand" : "Collapse"} ${node.label}`}
                  aria-expanded={!collapsed.has(node.id)}
                  onclick={() => toggle(node.id)}
                  ><span
                    class="icon-[lucide--chevron-right] size-3.5 transition-transform duration-150 ease-out motion-reduce:transition-none"
                    class:rotate-90={!collapsed.has(node.id)}
                    aria-hidden="true"
                  ></span></button
                >
              {:else}<span
                  class="shrink-0 w-5 grid place-items-center text-sm text-base-content/60"
                  aria-hidden="true"
                  >{node.kind === "agent"
                    ? "◈"
                    : node.kind === "command"
                      ? "›_"
                      : "◇"}</span
                >{/if}
              <button
                class="grid gap-1 min-w-0 flex-1 bg-transparent border-0 p-1 overflow-hidden text-left cursor-pointer text-base-content"
                onclick={() => onselect(node.id)}
                aria-pressed={selected === node.id}
                title={`${node.label} (${node.kind}, ${activityStatus(node)})`}
                ><strong
                  class="leading-4 text-xs font-medium whitespace-nowrap overflow-hidden text-ellipsis"
                  >{node.label}</strong
                >
                {#if node.usage}<span
                    class={[
                      "block max-h-4 overflow-hidden leading-4",
                      rowHeight < 44 && "hidden",
                    ]}><Usage usage={node.usage} compact /></span
                  >{:else}<small
                    class={[
                      "text-xs leading-3 text-base-content/60",
                      rowHeight < 32 && "hidden",
                    ]}>{node.kind === "input" ? activityStatus(node) : node.kind}</small
                  >{/if}</button
              >
            </div>
            <div
              class={[
                "touch-none select-none relative border-b border-base-300 min-w-0",
                dragging ? "cursor-grabbing" : "cursor-grab",
                selected === node.id ? "bg-primary/10" : "even:bg-base-200/50",
              ]}
              data-timeline-lane
              data-timeline-plot
            >
              {#each ticks as tick (tick)}<span
                  class="absolute top-0 bottom-0 w-px bg-base-300 pointer-events-none"
                  style:left={`${((tick - view.start) / view.span) * 100}%`}
                  aria-hidden="true"
                ></span>{/each}
              {#if cursor >= 0 && cursor <= 100}<span
                  class={[
                    "absolute inset-y-0 w-px z-2 pointer-events-none",
                    running ? "bg-error" : "bg-primary/50",
                  ]}
                  style:left={`${cursor}%`}
                  aria-hidden="true"
                ></span>{/if}
              <TaskMessageMarkers messages={node.messages ?? []} {view} width={plotWidth} onselect={id => { selectedMessageId = id; }} />
              {#if bar}
                <button
                  class={[
                    "absolute min-w-1.5 flex items-center gap-2 overflow-hidden rounded-field border text-left inset-shadow-2xs inset-shadow-white/10 bg-linear-to-b from-white/5 to-black/5 hover:brightness-105",
                    "inset-y-2",
                    dragging ? "cursor-grabbing" : "cursor-pointer",
                    node.status === "failed" || node.status === "blocked"
                      ? "bg-error border-error text-error-content light:bg-rose-100 light:border-rose-200 light:text-rose-950"
                      : node.status === "interrupted"
                        ? "bg-neutral border-neutral text-neutral-content border-dashed light:bg-slate-100 light:border-slate-300 light:text-slate-800"
                        : node.kind === "agent"
                          ? "bg-secondary border-secondary text-secondary-content light:bg-violet-100 light:border-violet-200 light:text-violet-950"
                          : node.kind === "command"
                            ? "bg-accent border-accent text-accent-content light:bg-teal-100 light:border-teal-200 light:text-teal-950"
                            : node.kind === "input"
                              ? "bg-warning border-warning text-warning-content light:bg-amber-100 light:border-amber-200 light:text-amber-950"
                              : "bg-primary border-primary text-primary-content light:bg-blue-100 light:border-blue-200 light:text-blue-950",
                    selected === node.id &&
                      "outline-2 outline-base-content -outline-offset-2 z-2",
                    bar.clippedStart && "rounded-l-none border-l-0 pl-5",
                    bar.clippedEnd && "rounded-r-none border-r-0 pr-5",
                  ]}
                  data-kind={node.kind}
                  style:left={`${bar.left}%`}
                  style:width={`max(6px, ${bar.width}%)`}
                  aria-label={`${node.label}, ${activityStatus(node)}, starts at ${duration(node.startedAt)}, duration ${duration(end - node.startedAt)}`}
                  aria-pressed={selected === node.id}
                  onclick={() => onselect(node.id)}
                  title={`${node.label}\n${activityStatus(node)} · ${duration(end - node.startedAt)}\nStart: ${duration(node.startedAt)}${node.preview ? `\n${node.preview}` : ""}`}
                >
                  {#if node.segments}
                    <ConversationSegments segments={node.segments} start={node.startedAt} {end} {elapsed} {view} width={plotWidth} />
                  {/if}
                  {#if isActivityActive(node)}
                    <span class="relative ml-2 flex shrink-0 items-center" title="Active task">
                      <ActivityIndicator />
                    </span>
                  {/if}
                  {#if bar.clippedStart}
                    <span
                      class="absolute top-0 bottom-0 w-5 grid place-items-center bg-black/20 text-lg leading-none tracking-tighter pointer-events-none z-1 left-0 border-r border-r-current/40 border-dashed"
                      aria-hidden="true">‹‹</span
                    >
                  {/if}
                  {#if !node.segments}
                  <span
                    class="relative z-1 text-xs font-medium whitespace-nowrap overflow-hidden text-ellipsis min-w-0 flex-1"
                    class:pl-2={!isActivityActive(node)}
                    >{node.kind === "agent" && node.preview
                      ? node.preview
                      : bar.clippedStart
                        ? ""
                        : node.label}</span
                  >
                  {#if !bar.clippedEnd}
                    <span
                      class="relative z-1 pr-2 text-xs whitespace-nowrap opacity-90"
                      >{duration(end - node.startedAt)}</span
                    >
                  {:else}
                    <span
                      class="absolute top-0 bottom-0 w-5 grid place-items-center bg-black/20 text-lg leading-none tracking-tighter pointer-events-none z-1 right-0 border-l border-l-current/40 border-dashed"
                      aria-hidden="true">››</span
                    >
                  {/if}
                  {/if}
                </button>
              {/if}
            </div>
          </div>
          {#if node.children.length}
            <div
              class={[
                "col-span-2 grid grid-cols-subgrid transition-[grid-template-rows] duration-150 ease-out motion-reduce:transition-none",
                collapsed.has(node.id) ? "grid-rows-[0fr]" : "grid-rows-[1fr]",
              ]}
              data-timeline-branch={node.id}
              inert={collapsed.has(node.id)}
              aria-hidden={collapsed.has(node.id)}
            >
              <!-- Open branches let message markers bridge the boundary between rows. -->
              <div
                class={["col-span-2 grid grid-cols-subgrid min-h-0", collapsed.has(node.id) && "overflow-hidden"]}
              >
                {#each node.children as child (child.id)}
                  {@render branch(child, depth + 1)}
                {/each}
              </div>
            </div>
          {/if}
        {/snippet}
        {#each nodes as node (node.id)}
          {@render branch(node, 0)}
        {/each}
        <div class="pointer-events-none absolute inset-y-0 left-0 z-4 w-(--activity-width)">
          <ColumnResizer bind:width={activityWidth} storageKey="runling-width-timeline-activity" label="Timeline activity column width" />
        </div>
      </div>
    </div>
  </div>
  {#if overview}
    <div class="shrink-0 border-t border-base-300 bg-base-200 overflow-y-auto [scrollbar-gutter:stable]">
      <div class="grid grid-cols-[var(--activity-width)_minmax(0,1fr)]">
        <div class="px-3 py-2 text-xs text-base-content/60">Overview</div>
        <TimelineMinimap
          rows={miniRows}
          vertical={{ start: scrollTop, span: visibleHeight, total: contentHeight }}
          {view}
          elapsed={extent}
          {selected}
          onzoomstart={(anchor) => beginMiddleZoom(anchor)}
          onchange={(next, top) => {
            manual = next;
            board.scrollTop = top;
          }}
          onfit={fit}
        />
      </div>
    </div>
  {/if}
  {#if message}
    <TaskMessageInspector
      messages={rows.flatMap(row => row.node.messages ?? [])}
      selected={message}
      onselect={id => { selectedMessageId = id; }}
      onclose={() => { selectedMessageId = undefined; }}
    />
  {/if}
  {#if expanded && inspected}
    <div class="py-3.5 px-5 border-t border-t-base-300 shrink-0">
      <strong class="text-xs">{inspected.label}</strong><small
        class="ml-3 text-base-content/60 text-xs">{inspected.status}</small
      >
      {#if inspected.usage}<Usage usage={inspected.usage} detail />{/if}
      <pre
        class="font-mono max-h-32.5 overflow-auto text-xs leading-relaxed whitespace-pre-wrap wrap-anywhere mb-0"><AnsiText
          text={inspected.logs.length
            ? inspected.logs.join("\n\n")
            : "No logs for this activity."}
        /></pre>
    </div>
  {/if}
  <footer
    class="flex flex-wrap justify-between gap-y-1.5 gap-x-4 py-2.5 px-3 border-t border-t-base-300 text-xs text-base-content/60 bg-base-100"
  >
    <span
      >Scroll to zoom time · Middle-drag ↔ time / ↕ rows · Left-drag to pan</span
    ><span class="tabular-nums"
      >{duration(view.start)} — {duration(view.start + view.span)}</span
    >
  </footer>
</section>
