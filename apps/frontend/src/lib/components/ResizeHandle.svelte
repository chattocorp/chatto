<script lang="ts">
  import * as m from '$lib/i18n/messages';

  let {
    width,
    min,
    max,
    onResize,
    onReset,
    edge = 'right',
    label = m['ui.resize_handle.resize']()
  }: {
    width: number;
    min: number;
    max: number;
    onResize: (newWidth: number) => void;
    onReset?: () => void;
    edge?: 'left' | 'right';
    label?: string;
  } = $props();

  let dragging = $state(false);

  function clamp(v: number): number {
    return Math.min(max, Math.max(min, v));
  }

  function onPointerDown(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    const startX = e.clientX;
    const startWidth = width;
    const sign = edge === 'right' ? 1 : -1;
    dragging = true;
    document.body.dataset.resizingSidebar = 'true';

    const onMove = (ev: PointerEvent) => {
      onResize(clamp(startWidth + sign * (ev.clientX - startX)));
    };

    const onUp = (ev: PointerEvent) => {
      target.releasePointerCapture(ev.pointerId);
      target.removeEventListener('pointermove', onMove);
      target.removeEventListener('pointerup', onUp);
      target.removeEventListener('pointercancel', onUp);
      delete document.body.dataset.resizingSidebar;
      dragging = false;
    };

    target.addEventListener('pointermove', onMove);
    target.addEventListener('pointerup', onUp);
    target.addEventListener('pointercancel', onUp);
  }

  function onDoubleClick() {
    onReset?.();
  }

  function onKeyDown(e: KeyboardEvent) {
    const step = e.shiftKey ? 32 : 8;
    const sign = edge === 'right' ? 1 : -1;
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      onResize(clamp(width - sign * step));
    } else if (e.key === 'ArrowRight') {
      e.preventDefault();
      onResize(clamp(width + sign * step));
    } else if (e.key === 'Home') {
      e.preventDefault();
      onResize(min);
    } else if (e.key === 'End') {
      e.preventDefault();
      onResize(max);
    }
  }
</script>

<button
  type="button"
  aria-label={label}
  class={[
    'group pointer-events-none absolute top-0 bottom-0 z-10 hidden w-6 cursor-col-resize touch-none border-0 bg-transparent p-0 md:block',
    edge === 'right' ? 'right-0' : 'left-0'
  ]}
  onpointerdown={onPointerDown}
  ondblclick={onDoubleClick}
  onkeydown={onKeyDown}
>
  <span
    data-testid="resize-handle-hit-target"
    class={[
      'pointer-events-auto absolute top-0 bottom-0 w-2',
      edge === 'right' ? 'right-0' : 'left-0'
    ]}
  >
    <span
      data-testid="resize-handle-line"
      class={[
        'pointer-events-none absolute top-0 bottom-0 w-px transition-colors',
        edge === 'right' ? 'right-0' : 'left-0',
        dragging
          ? 'bg-neutral-action'
          : 'bg-transparent group-hover:bg-neutral-action/60 group-focus-visible:bg-neutral-action'
      ]}
    ></span>
  </span>
</button>
