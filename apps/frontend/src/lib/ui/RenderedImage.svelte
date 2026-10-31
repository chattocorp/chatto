<!-- @component
Fitted SVG/Mermaid preview. It leaves conversation scrolling alone and opens the
shared attachment viewer for pan and zoom. The owner retains image/theme loading.
-->
<script lang="ts">
  import { on } from 'svelte/events';
  import { m } from '$lib/i18n/messages';
  import AttachmentModal from './AttachmentModal.svelte';
  import ZoomableImage from './ZoomableImage.svelte';

  let { image }: { image: HTMLImageElement } = $props();
  let width = $state(1);
  let height = $state(1);
  let ready = $state(false);
  let viewer = $state<{ src: string; alt: string } | null>(null);
  let viewerError = $state(false);
  let portalReady = $state(false);
  // Constrain width from the height budget, so both axes keep the image's ratio.
  // The minimum target size adds a small inset only for tiny or extreme artwork.
  const previewWidth = $derived(`min(${width}px, calc(min(30rem, 60vh) * ${width / height}))`);

  function preview(node: HTMLSpanElement) {
    const update = () => {
      width = image.naturalWidth || 1;
      height = image.naturalHeight || 1;
      ready = image.naturalWidth > 0;
      // The minimum click target must not enlarge tiny artwork inside it.
      image.style.maxWidth = `${width}px`;
      image.style.maxHeight = `${height}px`;
    };
    image.classList.add('image-canvas-asset');
    image.dataset.canvasImage = '';
    image.draggable = false;
    node.append(image);
    image.addEventListener('load', update);
    if (image.complete) update();
    return () => {
      image.removeEventListener('load', update);
      image.remove();
    };
  }

  /** Keep image activation out of message gestures, but preserve native scrolling. */
  function consumeMessageGestures(node: HTMLButtonElement) {
    const cleanups = ['mousedown', 'mouseup', 'touchstart', 'touchend'].map((type) =>
      on(node, type, (event) => event.stopPropagation())
    );
    return () => cleanups.forEach((cleanup) => cleanup());
  }

  function openViewer(event: MouseEvent) {
    event.stopPropagation();
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

<button
  type="button"
  class="rendered-image relative mx-auto block max-h-[min(30rem,60vh)] min-h-11 max-w-full min-w-11 cursor-zoom-in overflow-hidden p-0 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-action disabled:cursor-default"
  aria-label={m('ui.image_modal.open_larger')}
  disabled={!ready}
  onclick={openViewer}
  {@attach consumeMessageGestures}
  style:width={previewWidth}
  style:aspect-ratio={`${width} / ${height}`}
>
  <span class="absolute inset-0 flex items-center justify-center" {@attach preview}></span>
</button>

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
