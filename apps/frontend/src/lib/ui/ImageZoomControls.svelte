<!-- @component
Shared zoom toolbar. The viewer keeps it mounted while image stages change.
Without an active stage, controls show Fit and remain disabled.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { ImageZoomController } from './imageControlsContext';
  let { controls }: { controls?: ImageZoomController } = $props();
</script>

<div class="flex shrink-0 items-center gap-1 bg-background p-1 text-xs text-text">
  <button
    type="button"
    class="mini-icon-action"
    aria-label={m('ui.image_modal.zoom_out')}
    title={m('ui.image_modal.zoom_out')}
    disabled={!controls || controls.atMinimum}
    onclick={() => controls?.zoomOut()}
  >
    <span class="iconify icon-[uil--minus] text-xl" aria-hidden="true"></span>
  </button>
  <output class="w-14 text-center tabular-nums" aria-live="polite"
    >{controls?.percent ?? 100}%</output
  >
  <button
    type="button"
    class="mini-icon-action"
    aria-label={m('ui.image_modal.zoom_in')}
    title={m('ui.image_modal.zoom_in')}
    disabled={!controls || controls.atMaximum}
    onclick={() => controls?.zoomIn()}
  >
    <span class="iconify icon-[uil--plus] text-xl" aria-hidden="true"></span>
  </button>
  <button
    type="button"
    class="ms-2 mini-icon-action px-1"
    aria-label={m('ui.image_modal.fit')}
    title={m('ui.image_modal.fit')}
    disabled={!controls || controls.atFit}
    onclick={() => controls?.fit()}
  >
    {m('ui.image_modal.fit')}
  </button>
</div>
