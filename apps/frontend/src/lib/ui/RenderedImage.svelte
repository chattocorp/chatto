<!-- @component
Fitted SVG/Mermaid preview. It leaves conversation scrolling alone and opens the
shared attachment viewer for pan and zoom. The owner retains image/theme loading.
-->
<script lang="ts">
  import { on } from 'svelte/events';
  import { pushState } from '$app/navigation';
  import { m } from '$lib/i18n/messages';
  import { renderedImageWidth, type RenderedImageSize } from '$lib/renderedImageSize';

  let {
    image,
    initialSize
  }: {
    image: HTMLImageElement;
    /** Reserve the previous intrinsic size while a remounted image loads. */
    initialSize?: RenderedImageSize;
  } = $props();
  const descriptionId = $props.id();
  let width = $state(1);
  let height = $state(1);
  let ready = $state(false);
  // Constrain width from the height budget, so both axes keep the image's ratio.
  // The minimum target size adds a small inset only for tiny or extreme artwork.
  const previewWidth = $derived(renderedImageWidth({ width, height }));

  function preview(node: HTMLSpanElement) {
    const update = () => {
      const nextWidth = image.naturalWidth || initialSize?.width || 1;
      const nextHeight = image.naturalHeight || initialSize?.height || 1;
      width = nextWidth;
      height = nextHeight;
      ready = image.naturalWidth > 0;
      // The minimum click target must not enlarge tiny artwork inside it.
      image.style.maxWidth = `${nextWidth}px`;
      image.style.maxHeight = `${nextHeight}px`;
    };
    image.classList.add('image-canvas-asset');
    image.dataset.canvasImage = '';
    image.draggable = false;
    node.append(image);
    image.addEventListener('load', update);
    update();
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
    // Snapshot the rendered image; opening never uploads it or reparses its source.
    pushState('', { modal: { type: 'renderedImageViewer', src: image.src, alt: image.alt } });
  }
</script>

<button
  type="button"
  class="rendered-image relative rendered-image-size cursor-zoom-in overflow-hidden p-0 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-action disabled:cursor-default"
  aria-label={m('ui.image_modal.open_larger')}
  aria-describedby={descriptionId}
  disabled={!ready}
  onclick={openViewer}
  {@attach consumeMessageGestures}
  style:width={previewWidth}
  style:aspect-ratio={`${width} / ${height}`}
>
  <span class="absolute inset-0 flex items-center justify-center" {@attach preview}></span>
</button>
<span id={descriptionId} class="sr-only">{image.alt}</span>
