<!-- @component
Attachment viewer image stage with wheel/pinch zoom, bounded panning and Fit.
Controls live in the viewer footer when one is provided through context.
-->
<script module lang="ts">
  /** Allow shrinking below the fitted size without making the image disappear. */
  const MIN_ZOOM = 0.25;
  /** Maximum magnification relative to the fitted image. */
  const MAX_ZOOM = 16;
</script>

<script lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import { getContext } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import { imageControlsContext } from './AttachmentModal.svelte';

  let {
    src = '',
    alt = '',
    onerror
  }: {
    src?: string;
    alt?: string;
    onerror: () => void;
  } = $props();

  let stage: HTMLDivElement;
  const viewerControls = getContext<(() => HTMLElement | undefined) | undefined>(
    imageControlsContext
  );

  /** Relocate the one shared toolbar; its lifecycle and state stay with this canvas. */
  function placeControls(node: HTMLDivElement) {
    const target = viewerControls?.();
    if (!target) return;
    target.prepend(node);
    return () => node.remove();
  }
  let zoom = $state(1);
  let offsetX = $state(0);
  let offsetY = $state(0);
  let naturalWidth = $state(0);
  let naturalHeight = $state(0);
  let stageWidth = $state(0);
  let stageHeight = $state(0);
  // Pointer positions stay relative to the stage while it owns pointer capture.
  const pointers = new SvelteMap<number, { x: number; y: number }>();
  const percent = $derived(Math.round(zoom * 100));
  const limits = $derived(bounds());

  /** Limit panning to the fitted image's enlarged edges. */
  function bounds(atZoom = zoom) {
    const width = stageWidth || stage?.clientWidth || 0;
    const height = stageHeight || stage?.clientHeight || 0;
    if (!naturalWidth || !naturalHeight || !width || !height) return { x: 0, y: 0 };
    const fit = Math.min(width / naturalWidth, height / naturalHeight);
    return {
      x: Math.max(0, (naturalWidth * fit * atZoom - width) / 2),
      y: Math.max(0, (naturalHeight * fit * atZoom - height) / 2)
    };
  }

  function clampPan(x: number, y: number, atZoom = zoom) {
    const limit = bounds(atZoom);
    offsetX = Math.max(-limit.x, Math.min(limit.x, x));
    offsetY = Math.max(-limit.y, Math.min(limit.y, y));
  }

  /** Adjust the offset so the stage point under the cursor stays fixed. */
  function zoomAt(requested: number, clientX?: number, clientY?: number) {
    const next = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, requested));
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

  /** Preserve the user's zoom and keep pan within the resized stage. */
  function measureStage() {
    stageWidth = stage.clientWidth;
    stageHeight = stage.clientHeight;
    if (!stageWidth || !stageHeight || !naturalWidth || !naturalHeight) return;
    clampPan(offsetX, offsetY);
  }

  function measure(node: HTMLDivElement) {
    const observer = new ResizeObserver(measureStage);
    observer.observe(node);
    return () => observer.disconnect();
  }

  function measureImage(image: HTMLImageElement) {
    let frame = 0;
    const update = () => {
      naturalWidth = image.naturalWidth;
      naturalHeight = image.naturalHeight;
      // Image dimensions must reach layout before measuring bounds.
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(measureStage);
    };
    image.addEventListener('load', update);
    // A cached image can complete before its load listener is attached.
    if (image.complete) update();
    return () => {
      cancelAnimationFrame(frame);
      image.removeEventListener('load', update);
    };
  }

  function wheelZoom(node: HTMLDivElement) {
    function wheel(event: WheelEvent) {
      if (!naturalWidth || !naturalHeight) return;
      const unit =
        event.deltaMode === WheelEvent.DOM_DELTA_LINE
          ? 16
          : event.deltaMode === WheelEvent.DOM_DELTA_PAGE
            ? stageHeight
            : 1;
      zoomAt(zoom * Math.exp(-event.deltaY * unit * 0.002), event.clientX, event.clientY);
      event.preventDefault();
      event.stopPropagation();
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
    const active = stage?.closest('dialog')?.open;
    if (!active || event.altKey || event.ctrlKey || event.metaKey) return;
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

<div
  class="image-canvas relative flex h-full min-h-0 w-full flex-col bg-black text-white"
  data-sidebar-swipe-ignore
  style:--message-code-background="#000"
>
  <div
    bind:this={stage}
    {@attach measure}
    {@attach wheelZoom}
    {@attach gestureZoom}
    role="group"
    tabindex="-1"
    aria-label={m('ui.image_modal.fallback_alt')}
    dir="ltr"
    class="relative min-h-0 flex-1 touch-none overflow-hidden select-none"
    style:cursor={zoom > 1 ? (pointers.size ? 'grabbing' : 'grab') : 'zoom-in'}
    onpointerdown={pointerdown}
    onpointermove={pointermove}
    onpointerup={pointerend}
    onpointercancel={pointerend}
    onlostpointercapture={pointerend}
    ondblclick={(event) => zoomAt(zoom === 1 ? 2 : 1, event.clientX, event.clientY)}
  >
    <div
      class="image-canvas-layer absolute inset-0"
      style={`transform: translate(${offsetX}px, ${offsetY}px) scale(${zoom})`}
    >
      <img
        {src}
        {alt}
        draggable="false"
        data-canvas-image
        class="image-canvas-asset"
        {@attach measureImage}
        {onerror}
      />
    </div>
    <span
      aria-hidden="true"
      class={[
        'image-canvas-fade inset-x-0 top-0 h-8 bg-gradient-to-b',
        offsetY >= limits.y - 1 && 'opacity-0'
      ]}
    ></span>
    <span
      aria-hidden="true"
      class={[
        'image-canvas-fade inset-x-0 bottom-0 h-8 bg-gradient-to-t',
        offsetY <= -limits.y + 1 && 'opacity-0'
      ]}
    ></span>
    <span
      aria-hidden="true"
      class={[
        'image-canvas-fade inset-y-0 start-0 w-8 bg-gradient-to-r',
        offsetX >= limits.x - 1 && 'opacity-0'
      ]}
    ></span>
    <span
      aria-hidden="true"
      class={[
        'image-canvas-fade inset-y-0 end-0 w-8 bg-gradient-to-l',
        offsetX <= -limits.x + 1 && 'opacity-0'
      ]}
    ></span>
  </div>
  <div
    {@attach placeControls}
    class="flex shrink-0 items-center gap-1 bg-background p-1 text-xs text-text"
  >
    <button
      type="button"
      class="mini-icon-action"
      aria-label={m('ui.image_modal.zoom_out')}
      title={m('ui.image_modal.zoom_out')}
      disabled={zoom <= MIN_ZOOM}
      onclick={() => zoomAt(zoom - 0.25)}
    >
      <span class="iconify icon-[uil--minus] text-xl" aria-hidden="true"></span>
    </button>
    <output class="w-14 text-center tabular-nums" aria-live="polite">{percent}%</output>
    <button
      type="button"
      class="mini-icon-action"
      aria-label={m('ui.image_modal.zoom_in')}
      title={m('ui.image_modal.zoom_in')}
      disabled={zoom >= MAX_ZOOM}
      onclick={() => zoomAt(zoom + 0.25)}
    >
      <span class="iconify icon-[uil--plus] text-xl" aria-hidden="true"></span>
    </button>
    <button
      type="button"
      class="ms-2 mini-icon-action px-1"
      aria-label={m('ui.image_modal.fit')}
      title={m('ui.image_modal.fit')}
      disabled={zoom === 1}
      onclick={fit}
    >
      {m('ui.image_modal.fit')}
    </button>
  </div>
</div>
