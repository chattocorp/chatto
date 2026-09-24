<!-- @component A quiet loading surface with the same drifting light as the startup screen. Give it an explicit size for its context. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { ClassValue } from 'svelte/elements';
  import { fade } from 'svelte/transition';
  import { expoOutTransition } from './motion';

  let { class: className = '', label = m('common.loading') }: { class?: ClassValue; label?: string } =
    $props();

  function fadeOutFog(node: HTMLElement, bounds: DOMRect): void {
    const { duration } = expoOutTransition(120);
    if (!duration) return;

    if (!bounds.width || !bounds.height) return;

    // Fade a visual copy so the removed loading block cannot keep its parent content mounted.
    const ghost = node.cloneNode(false) as HTMLElement;
    ghost.removeAttribute('role');
    ghost.removeAttribute('aria-busy');
    ghost.removeAttribute('aria-label');
    ghost.removeAttribute('data-loading-fog');
    ghost.setAttribute('aria-hidden', 'true');
    ghost.style.position = 'fixed';
    ghost.style.inset = 'auto';
    ghost.style.left = `${bounds.left}px`;
    ghost.style.top = `${bounds.top}px`;
    ghost.style.width = `${bounds.width}px`;
    ghost.style.height = `${bounds.height}px`;
    ghost.style.margin = '0';
    ghost.style.pointerEvents = 'none';
    ghost.style.zIndex = '60';
    document.body.append(ghost);

    const animation = ghost.animate([{ opacity: 1 }, { opacity: 0 }], {
      duration,
      easing: 'ease-out',
      fill: 'forwards'
    });
    animation.onfinish = () => ghost.remove();
    animation.oncancel = () => ghost.remove();
  }

  function animateFog(node: HTMLElement): () => void {
    let removed = false;
    let stop: (() => void) | undefined;
    // Attachment cleanup runs after removal, so save the last visible frame.
    let bounds = node.getBoundingClientRect();
    const observer = new ResizeObserver(() => {
      bounds = node.getBoundingClientRect();
    });
    observer.observe(node);
    // Keep the static gradient while the optional motion code loads.
    void import('./fogGradients')
      .then(({ attachFogGradients }) => {
        if (!removed) stop = attachFogGradients(node);
      })
      .catch(() => undefined);
    return () => {
      observer.disconnect();
      fadeOutFog(node, bounds);
      removed = true;
      stop?.();
    };
  }
</script>

<div
  class={['loading-fog rounded-lg', className]}
  role="status"
  aria-busy="true"
  aria-label={label}
  data-loading-fog
  {@attach animateFog}
  in:fade|global={expoOutTransition(120)}
></div>
