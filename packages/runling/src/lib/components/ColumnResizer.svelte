<script lang="ts">
  import { onMount } from "svelte";

  let {
    width = $bindable<number | undefined>(undefined),
    storageKey,
    label,
    minWidth = 120,
    maxWidth = 600,
    class: className = "",
  }: {
    width?: number;
    storageKey: string;
    label: string;
    minWidth?: number;
    maxWidth?: number;
    class?: string;
  } = $props();

  let handle: HTMLDivElement;
  let actualWidth = $state<number>();
  let drag = $state<{ id: number; x: number; width: number } | null>(null);
  const clamp = (value: number) => Math.max(minWidth, Math.min(maxWidth, value));
  const measuredWidth = () => handle.parentElement!.getBoundingClientRect().width;

  function save() {
    try {
      if (width !== undefined) localStorage.setItem(storageKey, String(width));
    } catch {
      // Resizing also works when storage is unavailable.
    }
  }

  function finish(event: PointerEvent) {
    if (drag?.id !== event.pointerId) return;
    drag = null;
    if (handle.hasPointerCapture(event.pointerId)) handle.releasePointerCapture(event.pointerId);
    // Pointer focus must not leave a keyboard focus ring after the drag.
    handle.blur();
    save();
  }

  function keyboard(event: KeyboardEvent) {
    let next: number;
    const step = event.shiftKey ? 40 : 10;
    if (event.key === "ArrowLeft") next = measuredWidth() - step;
    else if (event.key === "ArrowRight") next = measuredWidth() + step;
    else if (event.key === "Home") next = minWidth;
    else if (event.key === "End") next = maxWidth;
    else return;
    event.preventDefault();
    event.stopPropagation();
    width = clamp(next);
    save();
  }

  onMount(() => {
    const observer = new ResizeObserver(() => {
      const measured = measuredWidth();
      if (measured > 0) actualWidth = measured;
    });
    observer.observe(handle.parentElement!);
    try {
      const saved = localStorage.getItem(storageKey);
      const value = saved === null ? NaN : Number(saved);
      if (Number.isFinite(value) && value > 0) width = clamp(value);
    } catch {
      // Keep the default width when storage is unavailable.
    }
    return () => observer.disconnect();
  });
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (A focusable separator implements keyboard resizing.) -->
<div
  bind:this={handle}
  role="separator"
  tabindex="0"
  aria-label={label}
  aria-orientation="vertical"
  aria-valuemin={minWidth}
  aria-valuemax={maxWidth}
  aria-valuenow={Math.round(actualWidth ?? width ?? minWidth)}
  aria-valuetext={actualWidth === undefined ? "Default width" : `${Math.round(actualWidth)} pixels`}
  title={`${label}. Drag or use the arrow keys to resize.`}
  class={[
    "absolute inset-y-0 right-0 z-10 w-1.5 cursor-col-resize touch-none select-none pointer-events-auto hover:bg-primary/50 focus-visible:bg-primary/50 focus-visible:outline-2 focus-visible:outline-primary",
    drag && "bg-primary/50",
    className,
  ]}
  onpointerdown={(event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.stopPropagation();
    handle.focus({ preventScroll: true });
    handle.setPointerCapture(event.pointerId);
    drag = { id: event.pointerId, x: event.clientX, width: measuredWidth() };
  }}
  onpointermove={(event) => {
    if (drag?.id !== event.pointerId) return;
    width = clamp(drag.width + event.clientX - drag.x);
  }}
  onpointerup={finish}
  onpointercancel={finish}
  onlostpointercapture={finish}
  onkeydown={keyboard}
></div>
