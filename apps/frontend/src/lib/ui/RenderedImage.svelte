<!-- @component
Fitted SVG/Mermaid preview. It leaves conversation scrolling alone and opens the
shared attachment viewer for pan and zoom. The owner retains image/theme loading.
-->
<script lang="ts">
  import { on } from 'svelte/events';
  import { pushState } from '$app/navigation';
  import { m } from '$lib/i18n/messages';
  import { renderedImageWidth, type RenderedImageSize } from '$lib/renderedImageSize';
  import SvgAnimation from './SvgAnimation.svelte';

  let {
    image,
    initialSize,
    svgSource,
    onerror
  }: {
    image: HTMLImageElement;
    /** Reserve the previous intrinsic size while a remounted image loads. */
    initialSize?: RenderedImageSize;
    /** Sanitized source, present only for supported SVG animations. */
    svgSource?: string;
    /** Let the Markdown owner reveal source if the playback frame fails. */
    onerror?: () => void;
  } = $props();
  const descriptionId = $props.id();
  let width = $state(1);
  let height = $state(1);
  let ready = $state(false);
  // Constrain width from the height budget, so both axes keep the image's ratio.
  // Extreme ratios keep a minimum click target around the complete image.
  const previewWidth = $derived(renderedImageWidth({ width, height }));

  function preview(node: HTMLSpanElement) {
    const update = () => {
      const nextWidth = image.naturalWidth || initialSize?.width || 1;
      const nextHeight = image.naturalHeight || initialSize?.height || 1;
      width = nextWidth;
      height = nextHeight;
      ready = image.naturalWidth > 0;
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
  function consumeMessageGestures(node: HTMLElement) {
    const cleanups = ['mousedown', 'mouseup', 'touchstart', 'touchend'].map((type) =>
      on(node, type, (event) => event.stopPropagation())
    );
    return () => cleanups.forEach((cleanup) => cleanup());
  }

  function openViewer(event: MouseEvent) {
    event.stopPropagation();
    // Snapshot sanitized artwork; opening never uploads or reads the current message.
    pushState('', {
      modal: {
        type: 'renderedImageViewer',
        alt: image.alt,
        ...(svgSource ? { svgSource } : { src: image.src })
      }
    });
  }
</script>

{#if svgSource}
  <div
    class="rendered-image relative mx-auto block max-w-full min-w-11"
    {@attach consumeMessageGestures}
    style:width={previewWidth}
    style:aspect-ratio={ready ? undefined : `${width} / ${height}`}
  >
    <span class="hidden" {@attach preview}></span>
    {#if ready}
      <SvgAnimation
        source={svgSource}
        alt={image.alt}
        {width}
        {height}
        onopen={openViewer}
        {onerror}
      />
    {/if}
  </div>
{:else}
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
{/if}
