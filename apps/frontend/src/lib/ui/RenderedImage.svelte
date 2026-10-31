<!-- @component
Fitted SVG/Mermaid preview. It leaves conversation scrolling alone and opens the
shared attachment viewer for pan and zoom. The owner retains image/theme loading.
-->
<script lang="ts">
  import { on } from 'svelte/events';
  import { m } from '$lib/i18n/messages';
  import AttachmentModal from './AttachmentModal.svelte';
  import ZoomableImage from './ZoomableImage.svelte';

  let { image, controlsTarget }: { image: HTMLImageElement; controlsTarget: HTMLElement } =
    $props();
  let width = $state(1);
  let height = $state(1);
  let ready = $state(false);
  let viewer = $state<{ src: string; alt: string } | null>(null);
  let viewerError = $state(false);
  let portalReady = $state(false);

  function preview(node: HTMLDivElement) {
    const update = () => {
      width = image.naturalWidth || 1;
      height = image.naturalHeight || 1;
      ready = image.naturalWidth > 0;
    };
    image.classList.add('image-canvas-asset');
    image.dataset.canvasImage = '';
    node.append(image);
    image.addEventListener('load', update);
    if (image.complete) update();
    return () => {
      image.removeEventListener('load', update);
      image.remove();
    };
  }

  /** Actions live in the owner's preview visor or code header. */
  function actions(node: HTMLDivElement) {
    controlsTarget.prepend(node);
    const cleanups = ['click', 'mousedown', 'mouseup', 'touchstart', 'touchend'].map((type) =>
      on(node, type, (event) => event.stopPropagation())
    );
    return () => {
      cleanups.forEach((cleanup) => cleanup());
      node.remove();
    };
  }

  function openViewer() {
    viewerError = false;
    portalReady = false;
    // Snapshot the rendered image; opening never uploads it or reparses its source.
    viewer = { src: image.src, alt: image.alt };
  }

  /** Relocate before mounting the native dialog, preserving its modal top layer. */
  function portal(node: HTMLDivElement) {
    document.body.append(node);
    portalReady = true;
    return () => node.remove();
  }
</script>

<div
  class="rendered-image relative max-h-[min(30rem,60vh)] min-h-24 w-full overflow-hidden"
  style:aspect-ratio={`${width} / ${height}`}
>
  <div class="absolute inset-0" {@attach preview}></div>
</div>
<div class="me-auto flex items-center" data-inline-image-controls {@attach actions}>
  <button
    type="button"
    data-open-larger
    class="mini-icon-action h-5 gap-1 whitespace-nowrap"
    disabled={!ready}
    onclick={openViewer}
  >
    <span class="iconify icon-[uil--expand-arrows-alt] text-sm" aria-hidden="true"></span>
    {m('ui.image_modal.open_larger')}
  </button>
</div>

{#if viewer}
  <div class="font-sans text-base" {@attach portal}>
    {#if portalReady}
      <AttachmentModal
        filename="diagram.svg"
        contentType="image/svg+xml"
        description={viewer.alt}
        downloadUrl={viewer.src}
        imageViewer
        error={viewerError ? m('room.attachment.html_viewer.preview_failed') : null}
        ondownload={(event) => event.stopPropagation()}
        onclose={() => (viewer = null)}
      >
        <ZoomableImage src={viewer.src} alt={viewer.alt} onerror={() => (viewerError = true)} />
      </AttachmentModal>
    {/if}
  </div>
{/if}
