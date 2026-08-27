<script lang="ts">
  import { onDestroy } from 'svelte';
  import { m } from '$lib/i18n/messages';

  let {
    width,
    min,
    max,
    onResize,
    onReset,
    edge = 'end',
    label = m('ui.resize_handle.resize')
  }: {
    width: number;
    min: number;
    max: number;
    onResize: (newWidth: number) => void;
    onReset?: () => void;
    /** Logical edge of the sidebar that owns this handle. */
    edge?: 'start' | 'end';
    label?: string;
  } = $props();

  let dragging = $state(false);
  let activeDrag:
    | {
        pointerId: number;
        startX: number;
        startWidth: number;
        sign: number;
        target: HTMLElement;
      }
    | undefined;

  function clamp(v: number): number {
    return Math.min(max, Math.max(min, v));
  }

  function onPointerDown(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    finishDragging();
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    const rtl = document.documentElement.dir === 'rtl';
    const physicalEdge = edge === 'end' ? (rtl ? 'left' : 'right') : rtl ? 'right' : 'left';
    const sign = physicalEdge === 'right' ? 1 : -1;
    activeDrag = {
      pointerId: e.pointerId,
      startX: e.clientX,
      startWidth: width,
      sign,
      target
    };
    dragging = true;
    document.body.dataset.resizingSidebar = 'true';
  }

  function onWindowPointerMove(e: PointerEvent) {
    if (!activeDrag || e.pointerId !== activeDrag.pointerId) return;
    if ((e.buttons & 1) === 0) {
      finishDragging(e.pointerId);
      return;
    }
    onResize(clamp(activeDrag.startWidth + activeDrag.sign * (e.clientX - activeDrag.startX)));
  }

  function finishDragging(pointerId?: number) {
    if (!activeDrag || (pointerId !== undefined && pointerId !== activeDrag.pointerId)) return;
    const { target, pointerId: activePointerId } = activeDrag;
    activeDrag = undefined;
    dragging = false;
    delete document.body.dataset.resizingSidebar;

    if (target.hasPointerCapture(activePointerId)) {
      target.releasePointerCapture(activePointerId);
    }
  }

  function onDoubleClick() {
    onReset?.();
  }

  function onKeyDown(e: KeyboardEvent) {
    const step = e.shiftKey ? 32 : 8;
    const rtl = document.documentElement.dir === 'rtl';
    const physicalEdge = edge === 'end' ? (rtl ? 'left' : 'right') : rtl ? 'right' : 'left';
    const sign = physicalEdge === 'right' ? 1 : -1;
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      onResize(clamp(width - sign * step));
    } else if (e.key === 'ArrowRight') {
      e.preventDefault();
      onResize(clamp(width + sign * step));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      onResize(clamp(width + step));
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      onResize(clamp(width - step));
    } else if (e.key === 'PageUp') {
      e.preventDefault();
      onResize(clamp(width + 32));
    } else if (e.key === 'PageDown') {
      e.preventDefault();
      onResize(clamp(width - 32));
    } else if (e.key === 'Home') {
      e.preventDefault();
      onResize(min);
    } else if (e.key === 'End') {
      e.preventDefault();
      onResize(max);
    }
  }

  onDestroy(() => finishDragging());
</script>

<svelte:window
  onpointermove={onWindowPointerMove}
  onpointerup={(e) => finishDragging(e.pointerId)}
  onpointercancel={(e) => finishDragging(e.pointerId)}
  onblur={() => finishDragging()}
/>

<div
  data-testid="resize-handle"
  class={[
    'pointer-events-none absolute top-0 bottom-0 z-10 hidden w-6 md:block',
    edge === 'end' ? 'end-0' : 'start-0'
  ]}
>
  <div
    aria-hidden="true"
    data-sidebar-swipe-ignore
    data-testid="resize-handle-drag-strip"
    class={[
      'peer pointer-events-auto absolute top-0 bottom-0 h-full w-2 cursor-col-resize touch-none border-0 bg-transparent p-0',
      edge === 'end' ? 'end-0' : 'start-0'
    ]}
    onpointerdown={onPointerDown}
    onlostpointercapture={(e) => finishDragging(e.pointerId)}
    ondblclick={onDoubleClick}
  ></div>
  <div
    role="slider"
    aria-orientation="vertical"
    aria-label={label}
    aria-valuemin={min}
    aria-valuemax={max}
    aria-valuenow={Math.round(width)}
    tabindex="0"
    data-sidebar-swipe-ignore
    data-testid="resize-handle-hit-target"
    class={[
      'peer pointer-events-auto absolute top-1/2 h-6 w-6 -translate-y-1/2 cursor-col-resize touch-none border-0 bg-transparent p-0',
      edge === 'end' ? 'end-0' : 'start-0'
    ]}
    onpointerdown={onPointerDown}
    onlostpointercapture={(e) => finishDragging(e.pointerId)}
    ondblclick={onDoubleClick}
    onkeydown={onKeyDown}
  ></div>
  <span
    data-testid="resize-handle-line"
    class={[
      'pointer-events-none absolute top-0 bottom-0 w-px transition-colors',
      edge === 'end' ? 'end-0' : 'start-0',
      dragging
        ? 'bg-neutral-action'
        : 'bg-transparent peer-hover:bg-neutral-action/60 peer-focus-visible:bg-neutral-action'
    ]}
  ></span>
</div>
