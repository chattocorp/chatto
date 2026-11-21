<!-- @component
Plays a GLSL fragment shader from a Markdown code block on the shared WebGL
renderer in `$lib/glsl/renderer`. Playback follows the animated SVG rules:
reduced motion starts paused, and hidden or offscreen previews pause. A frame
that is too slow pauses the preview. The caller checks the shader before mount.
Pointer presses and drags on the canvas set `iMouse`. Touch drags still scroll
the conversation; the browser then cancels the press.
-->
<script lang="ts">
  import {
    createShaderPlayer,
    SHADER_HEIGHT,
    SHADER_WIDTH,
    type PointerKind
  } from '$lib/glsl/renderer';
  import { on } from 'svelte/events';
  import { motionPreference } from '$lib/dom/motionPreference';
  import { m } from '$lib/i18n/messages';
  import PreviewToolbar from './PreviewToolbar.svelte';
  import PreviewAction from './PreviewAction.svelte';

  let {
    source,
    onerror
  }: {
    /** Shadertoy-style fragment source that `isShaderUsable` accepted. */
    source: string;
    /** Let the Markdown owner reveal source if rendering fails. */
    onerror?: () => void;
  } = $props();

  let paused = $state(false);

  /** The player belongs to the canvas and stops when the preview unmounts. */
  function play(canvas: HTMLCanvasElement) {
    const motion = motionPreference();
    let visible = false;
    paused = motion.matches;
    const player = createShaderPlayer(source, canvas, {
      onerror: () => onerror?.(),
      onslow: () => (paused = true)
    });
    if (!player) return;
    const sync = () => player.setPlaying(!paused && visible && !document.hidden);
    const observer = new IntersectionObserver(([entry]) => {
      visible = entry?.isIntersecting ?? false;
      sync();
    });
    const motionChanged = () => {
      if (motion.matches) paused = true;
      sync();
    };
    /** Report a pointer position relative to the displayed canvas. */
    const report = (kind: PointerKind, event: PointerEvent) => {
      const rect = canvas.getBoundingClientRect();
      player.pointer(
        kind,
        (event.clientX - rect.left) / rect.width,
        (event.clientY - rect.top) / rect.height
      );
    };
    const pointerCleanups = [
      on(canvas, 'pointerdown', (event) => {
        if (!event.isPrimary || event.button !== 0) return;
        canvas.setPointerCapture(event.pointerId);
        report('press', event);
      }),
      on(canvas, 'pointermove', (event) => {
        if (event.isPrimary) report('move', event);
      }),
      on(canvas, 'pointerup', (event) => {
        if (event.isPrimary) report('release', event);
      }),
      on(canvas, 'pointercancel', (event) => {
        if (event.isPrimary) report('release', event);
      }),
      // Keep shader input out of the message row's selection and long-press gestures.
      ...['mousedown', 'mouseup', 'touchstart', 'touchend'].map((type) =>
        on(canvas, type, (event) => event.stopPropagation())
      )
    ];
    observer.observe(canvas);
    motion.addEventListener('change', motionChanged);
    document.addEventListener('visibilitychange', sync);
    $effect(() => {
      void paused;
      sync();
    });
    return () => {
      observer.disconnect();
      motion.removeEventListener('change', motionChanged);
      document.removeEventListener('visibilitychange', sync);
      pointerCleanups.forEach((cleanup) => cleanup());
      player.dispose();
    };
  }
</script>

<div class="flex w-xl max-w-full flex-col">
  <div role="img" aria-label={m('common.code_shader')}>
    <canvas
      class="shader-preview block aspect-video h-auto w-full cursor-crosshair"
      width={SHADER_WIDTH}
      height={SHADER_HEIGHT}
      aria-hidden="true"
      {@attach play}
    ></canvas>
  </div>
  <PreviewToolbar>
    {#snippet viewing()}
      <PreviewAction
        label={m('ui.image_modal.pause_animation')}
        aria-pressed={paused}
        icon={paused ? 'icon-[uil--play]' : 'icon-[uil--pause]'}
        onclick={() => (paused = !paused)}
      />
    {/snippet}
  </PreviewToolbar>
</div>
