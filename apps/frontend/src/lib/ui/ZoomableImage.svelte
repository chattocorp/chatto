<!-- @component
An image stage with local zoom and pan state. The parent keeps this component
mounted when a signed image URL refreshes and remounts it for another image.
-->
<script lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import { m } from '$lib/i18n/messages';
  import SkeletonImg from './SkeletonImg.svelte';

  let {
    src,
    alt,
    onerror
  }: {
    src: string;
    alt: string;
    onerror: () => void;
  } = $props();

  let stage: HTMLDivElement;
  let zoom = $state(1);
  let offsetX = $state(0);
  let offsetY = $state(0);
  let naturalWidth = 0;
  let naturalHeight = 0;
  let stageWidth = 0;
  let stageHeight = 0;
  // Pointer positions stay relative to the stage while it owns pointer capture.
  const pointers = new SvelteMap<number, { x: number; y: number }>();
  const percent = $derived(Math.round(zoom * 100));

  /** Limit panning to the fitted image's enlarged edges. */
  function bounds(atZoom = zoom) {
    if (!naturalWidth || !naturalHeight || !stageWidth || !stageHeight) return { x: 0, y: 0 };
    const fit = Math.min(stageWidth / naturalWidth, stageHeight / naturalHeight);
    return {
      x: Math.max(0, (naturalWidth * fit * atZoom - stageWidth) / 2),
      y: Math.max(0, (naturalHeight * fit * atZoom - stageHeight) / 2)
    };
  }

  function clampPan(x: number, y: number, atZoom = zoom) {
    const limit = bounds(atZoom);
    offsetX = Math.max(-limit.x, Math.min(limit.x, x));
    offsetY = Math.max(-limit.y, Math.min(limit.y, y));
  }

  /** Adjust the offset so the stage point under the cursor stays fixed. */
  function zoomAt(requested: number, clientX?: number, clientY?: number) {
    const next = Math.max(1, Math.min(4, requested));
    if (next === zoom) return;
    const rect = stage.getBoundingClientRect();
    const x = (clientX ?? rect.left + rect.width / 2) - rect.left - rect.width / 2;
    const y = (clientY ?? rect.top + rect.height / 2) - rect.top - rect.height / 2;
    const ratio = next / zoom;
    const nextX = offsetX + (1 - ratio) * (x - offsetX);
    const nextY = offsetY + (1 - ratio) * (y - offsetY);
    zoom = next;
    clampPan(nextX, nextY, next);
  }

  function fit() {
    zoom = 1;
    offsetX = 0;
    offsetY = 0;
  }

  function measure(node: HTMLDivElement) {
    const observer = new ResizeObserver(() => {
      stageWidth = node.clientWidth;
      stageHeight = node.clientHeight;
      clampPan(offsetX, offsetY);
    });
    observer.observe(node);
    return () => observer.disconnect();
  }

  function wheelZoom(node: HTMLDivElement) {
    function wheel(event: WheelEvent) {
      if (!naturalWidth || !naturalHeight) return;
      event.preventDefault();
      const delta = event.deltaY * (event.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : 1);
      zoomAt(zoom * Math.exp(-delta * 0.002), event.clientX, event.clientY);
    }
    node.addEventListener('wheel', wheel, { passive: false });
    return () => node.removeEventListener('wheel', wheel);
  }

  /** Safari sends trackpad pinches as gesture events instead of wheel events. */
  function gestureZoom(node: HTMLDivElement) {
    let startingZoom = 1;
    function start(event: Event) {
      event.preventDefault();
      startingZoom = zoom;
    }
    function change(event: Event) {
      event.preventDefault();
      const gesture = event as Event & { scale: number; clientX?: number; clientY?: number };
      zoomAt(startingZoom * gesture.scale, gesture.clientX, gesture.clientY);
    }
    node.addEventListener('gesturestart', start);
    node.addEventListener('gesturechange', change);
    return () => {
      node.removeEventListener('gesturestart', start);
      node.removeEventListener('gesturechange', change);
    };
  }

  function position(event: PointerEvent) {
    const rect = stage.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
  }

  function pointerdown(event: PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return;
    pointers.set(event.pointerId, position(event));
    stage.setPointerCapture(event.pointerId);
  }

  function pointermove(event: PointerEvent) {
    const previous = pointers.get(event.pointerId);
    if (!previous) return;
    const next = position(event);
    if (pointers.size === 1) {
      if (zoom > 1) clampPan(offsetX + next.x - previous.x, offsetY + next.y - previous.y);
    } else if (pointers.size === 2) {
      const other = [...pointers].find(([id]) => id !== event.pointerId)?.[1];
      if (other) {
        const oldDistance = Math.hypot(previous.x - other.x, previous.y - other.y);
        const newDistance = Math.hypot(next.x - other.x, next.y - other.y);
        const oldX = (previous.x + other.x) / 2;
        const oldY = (previous.y + other.y) / 2;
        const newX = (next.x + other.x) / 2;
        const newY = (next.y + other.y) / 2;
        const rect = stage.getBoundingClientRect();
        if (oldDistance)
          zoomAt((zoom * newDistance) / oldDistance, rect.left + oldX, rect.top + oldY);
        clampPan(offsetX + newX - oldX, offsetY + newY - oldY);
      }
    }
    pointers.set(event.pointerId, next);
  }

  function pointerend(event: PointerEvent) {
    pointers.delete(event.pointerId);
    if (stage.hasPointerCapture(event.pointerId)) stage.releasePointerCapture(event.pointerId);
  }

  function keydown(event: KeyboardEvent) {
    if (!stage?.closest('dialog')?.open || event.altKey || event.ctrlKey || event.metaKey) return;
    if (
      event.target instanceof Element &&
      event.target.closest('input,textarea,select,[contenteditable]')
    )
      return;
    if (event.key === '+' || event.key === '=') zoomAt(zoom + 0.25);
    else if (event.key === '-') zoomAt(zoom - 0.25);
    else if (event.key === '0') fit();
    else return;
    event.preventDefault();
  }
</script>

<svelte:window onkeydown={keydown} />

<div class="flex h-full min-h-0 w-full flex-col bg-black text-white">
  <div
    bind:this={stage}
    {@attach measure}
    {@attach wheelZoom}
    {@attach gestureZoom}
    role="group"
    aria-label={m('ui.image_modal.fallback_alt')}
    class="relative min-h-0 flex-1 touch-none overflow-hidden select-none"
    style:cursor={zoom > 1 ? (pointers.size ? 'grabbing' : 'grab') : 'zoom-in'}
    onpointerdown={pointerdown}
    onpointermove={pointermove}
    onpointerup={pointerend}
    onpointercancel={pointerend}
    ondblclick={(event) => zoomAt(zoom === 1 ? 2 : 1, event.clientX, event.clientY)}
  >
    <SkeletonImg
      {src}
      {alt}
      draggable="false"
      class="pointer-events-none h-full w-full object-contain outline-none"
      style={`transform: translate(${offsetX}px, ${offsetY}px) scale(${zoom})`}
      onload={(event) => {
        const image = event.currentTarget as HTMLImageElement;
        naturalWidth = image.naturalWidth;
        naturalHeight = image.naturalHeight;
        clampPan(offsetX, offsetY);
      }}
      {onerror}
    />
  </div>
  <div class="flex shrink-0 items-center justify-center gap-1 px-2 py-1">
    <button
      type="button"
      class="icon-action text-white"
      aria-label={m('ui.image_modal.zoom_out')}
      disabled={zoom <= 1}
      onclick={() => zoomAt(zoom - 0.25)}
    >
      <span class="iconify icon-[uil--minus] text-xl" aria-hidden="true"></span>
    </button>
    <output class="w-14 text-center tabular-nums" aria-live="polite">{percent}%</output>
    <button
      type="button"
      class="icon-action text-white"
      aria-label={m('ui.image_modal.zoom_in')}
      disabled={zoom >= 4}
      onclick={() => zoomAt(zoom + 0.25)}
    >
      <span class="iconify icon-[uil--plus] text-xl" aria-hidden="true"></span>
    </button>
    <button type="button" class="ms-2 btn-secondary" disabled={zoom === 1} onclick={fit}
      >{m('ui.image_modal.fit')}</button
    >
  </div>
</div>
