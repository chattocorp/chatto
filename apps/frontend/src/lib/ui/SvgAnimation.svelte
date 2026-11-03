<!-- @component
Plays a sanitized SVG in a frame with scripts and resource loads disabled.
The parent controls the SVG timeline. Offscreen and hidden frames pause work.
-->
<script lang="ts">
  import { SVG_ANIMATION_FRAME_DOCUMENT } from '$lib/svgAnimationFrame';
  import { motionPreference } from '$lib/dom/motionPreference';
  import { m } from '$lib/i18n/messages';
  import PreviewToolbar from './PreviewToolbar.svelte';
  import PreviewAction from './PreviewAction.svelte';
  import { hasImageControls, useImageControls } from './imageControlsContext';

  let {
    source,
    alt,
    width,
    height,
    onopen,
    onerror
  }: {
    /** Only SVG returned by sanitizeSvg may enter this renderer. */
    source: string;
    alt: string;
    width: number;
    height: number;
    onopen?: (event: MouseEvent) => void;
    onerror?: () => void;
  } = $props();

  const viewerControls = hasImageControls() ? useImageControls() : undefined;
  let svg: SVGSVGElement | undefined;
  let visible = true;
  let ready = $state(false);
  let paused = $state(false);
  const descriptionId = $props.id();

  function syncPlayback() {
    if (!svg) return;
    if (paused || !visible || document.hidden) svg.pauseAnimations();
    else svg.unpauseAnimations();
  }

  /** Replace artwork when its source changes, after the fixed frame policy loads. */
  function content(frame: HTMLIFrameElement) {
    const parsed = new DOMParser().parseFromString(source, 'image/svg+xml');
    const parsedRoot = parsed.querySelector('svg');
    if (
      !parsedRoot ||
      parsedRoot.parentNode !== parsed ||
      parsedRoot.namespaceURI !== 'http://www.w3.org/2000/svg'
    ) {
      onerror?.();
      return;
    }
    const fallbackViewBox = `0 0 ${width} ${height}`;
    const failed = onerror;
    ready = false;
    const load = () => {
      const document = frame.contentDocument;
      if (!document) {
        failed?.();
        return;
      }
      const root = document.importNode(parsedRoot, true);
      // A viewBox makes fixed dimensions fit the existing image stage.
      if (!root.hasAttribute('viewBox')) root.setAttribute('viewBox', fallbackViewBox);
      document.body.replaceChildren(root);
      svg = root;
      const startPaused = motionPreference().matches;
      paused = startPaused;
      if (startPaused) svg.setCurrentTime(0);
      ready = true;
      // Keep initial setup independent of the reactive Pause button state.
      if (startPaused || !visible || window.document.hidden) root.pauseAnimations();
      else root.unpauseAnimations();
    };
    frame.addEventListener('load', load);
    // A new source can reuse a frame whose restrictive policy is already active.
    if (frame.contentDocument?.querySelector('meta[http-equiv="Content-Security-Policy"]')) load();
    return () => {
      frame.removeEventListener('load', load);
      svg?.pauseAnimations();
      svg = undefined;
    };
  }

  /** Observers belong to the frame and are removed when its preview is disposed. */
  function observe(frame: HTMLIFrameElement) {
    const motion = motionPreference();
    const changed = () => {
      if (motion.matches) paused = true;
      syncPlayback();
    };
    const observer = new IntersectionObserver(([entry]) => {
      visible = entry?.isIntersecting ?? false;
      syncPlayback();
    });
    observer.observe(frame);
    motion.addEventListener('change', changed);
    return () => {
      observer.disconnect();
      motion.removeEventListener('change', changed);
      svg?.pauseAnimations();
      svg = undefined;
    };
  }

  /** Put viewer playback beside zoom controls, outside the transformed stage. */
  function placeControls(node: HTMLDivElement) {
    const target = viewerControls?.();
    if (!target) return;
    target.append(node);
    return () => node.remove();
  }
</script>

<svelte:document onvisibilitychange={syncPlayback} />

<div class="group/preview relative h-full w-full" style:--preview-toolbar-inset="0px">
  <iframe
    title={alt}
    sandbox="allow-same-origin"
    srcdoc={SVG_ANIMATION_FRAME_DOCUMENT}
    tabindex="-1"
    aria-hidden="true"
    class="pointer-events-none absolute inset-0 h-full w-full border-0 [color-scheme:normal]"
    {@attach content}
    {@attach observe}
  ></iframe>
  {#if onopen}
    <button
      type="button"
      class="rendered-image absolute inset-0 h-full w-full cursor-zoom-in focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-action"
      aria-label={m('ui.image_modal.open_larger')}
      aria-describedby={descriptionId}
      disabled={!ready}
      onclick={onopen}
    ></button>
  {/if}
  <span id={descriptionId} class="sr-only">{alt}</span>
  <div {@attach placeControls}>
    <PreviewToolbar layout={viewerControls ? 'row' : 'overlay'}>
      <PreviewAction
        label={m(paused ? 'ui.image_modal.resume_animation' : 'ui.image_modal.pause_animation')}
        icon={paused ? 'icon-[uil--play]' : 'icon-[uil--pause]'}
        disabled={!ready}
        onclick={() => {
          paused = !paused;
          syncPlayback();
        }}
      />
    </PreviewToolbar>
  </div>
</div>
