<!-- @component
Shared image stage for the attachment viewer and inline SVG/Mermaid previews.
Inline stages fill their frame, start fitted and pan on normal wheel
input. Viewer stages zoom on wheel input. Both share pinch, drag, Fit and bounds.
-->
<script module lang="ts">
  /** Maximum magnification relative to the fitted image. */
  const MAX_ZOOM = 16;
</script>

<script lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import { on } from 'svelte/events';
  import { m } from '$lib/i18n/messages';

  let {
    src = '',
    alt = '',
    image,
    inline = false,
    onerror
  }: {
    src?: string;
    alt?: string;
    /** Reuse an existing image, including its theme updates and load listeners. */
    image?: HTMLImageElement;
    inline?: boolean;
    onerror: () => void;
  } = $props();

  let stage: HTMLDivElement;
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
    const next = Math.max(1, Math.min(MAX_ZOOM, requested));
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
      // Inline aspect ratio changes must reach layout before measuring bounds.
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

  function existingImage(node: HTMLDivElement) {
    if (!image) return;
    image.classList.add('image-canvas-asset');
    image.dataset.canvasImage = '';
    image.draggable = false;
    node.append(image);
    const cleanup = measureImage(image);
    return () => {
      cleanup();
      image.remove();
    };
  }

  /** Consume message activation gestures after the canvas controls handle them. */
  function containMessageGestures(node: HTMLDivElement) {
    if (!inline) return;
    const cleanups = (
      ['click', 'dblclick', 'mousedown', 'mouseup', 'touchstart', 'touchend'] as const
    ).map((type) => on(node, type, (event) => event.stopPropagation()));
    return () => cleanups.forEach((cleanup) => cleanup());
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
      if (inline && !event.ctrlKey && !event.metaKey) {
        const previousX = offsetX;
        const previousY = offsetY;
        const horizontal = event.shiftKey && !event.deltaX;
        clampPan(
          offsetX - (horizontal ? event.deltaY : event.deltaX) * unit,
          offsetY - (horizontal ? 0 : event.deltaY) * unit
        );
        // At an edge, leave the wheel to the conversation's native scrolling.
        if (offsetX === previousX && offsetY === previousY) return;
      } else zoomAt(zoom * Math.exp(-event.deltaY * unit * 0.002), event.clientX, event.clientY);
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
    if (inline) stage.focus({ preventScroll: true });
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
    const active = inline
      ? stage?.parentElement?.contains(document.activeElement)
      : stage?.closest('dialog')?.open;
    if (!active || event.altKey || event.ctrlKey || event.metaKey) return;
    if (
      event.target instanceof Element &&
      event.target.closest('input,textarea,select,[contenteditable]')
    )
      return;
    if (event.key === '+' || event.key === '=') zoomAt(zoom + 0.25);
    else if (event.key === '-') zoomAt(zoom - 0.25);
    else if (event.key === '0') fit();
    else if (inline && event.key === 'ArrowDown') clampPan(offsetX, offsetY - 40);
    else if (inline && event.key === 'ArrowUp') clampPan(offsetX, offsetY + 40);
    else if (inline && event.key === 'ArrowRight') clampPan(offsetX - 40, offsetY);
    else if (inline && event.key === 'ArrowLeft') clampPan(offsetX + 40, offsetY);
    else return;
    event.preventDefault();
  }
</script>

<svelte:window onkeydown={keydown} />

<div
  class={[
    'image-canvas relative flex min-h-0 w-full flex-col',
    inline ? 'mx-auto text-text' : 'h-full bg-black text-white'
  ]}
  data-sidebar-swipe-ignore
  {@attach containMessageGestures}
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
    class={[
      'relative min-h-0 touch-none overflow-hidden select-none',
      inline ? 'max-h-[min(30rem,60vh)] w-full' : 'flex-1'
    ]}
    style:aspect-ratio={inline ? `${naturalWidth || 1} / ${naturalHeight || 1}` : undefined}
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
      {#if image}
        <div class="contents" {@attach existingImage}></div>
      {:else}
        <img
          {src}
          {alt}
          draggable="false"
          data-canvas-image
          class="image-canvas-asset"
          {@attach measureImage}
          {onerror}
        />
      {/if}
    </div>
    {#if inline}
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
    {/if}
  </div>
  <div
    class={[
      'flex items-center justify-center gap-1 px-2 py-1',
      inline
        ? 'absolute inset-x-2 bottom-2 z-40 mx-auto w-fit max-w-full shell-surface text-xs'
        : 'shrink-0'
    ]}
  >
    <button
      type="button"
      class={inline ? 'mini-icon-action' : 'icon-action text-white'}
      aria-label={m('ui.image_modal.zoom_out')}
      disabled={zoom <= 1}
      onclick={() => zoomAt(zoom - 0.25)}
    >
      <span class="iconify icon-[uil--minus] text-xl" aria-hidden="true"></span>
    </button>
    <output class="w-14 text-center tabular-nums" aria-live="polite">{percent}%</output>
    <button
      type="button"
      class={inline ? 'mini-icon-action' : 'icon-action text-white'}
      aria-label={m('ui.image_modal.zoom_in')}
      disabled={zoom >= MAX_ZOOM}
      onclick={() => zoomAt(zoom + 0.25)}
    >
      <span class="iconify icon-[uil--plus] text-xl" aria-hidden="true"></span>
    </button>
    <button
      type="button"
      class={['ms-2 btn-secondary', inline && 'btn-compact']}
      disabled={zoom === 1}
      onclick={fit}>{m('ui.image_modal.fit')}</button
    >
  </div>
</div>
