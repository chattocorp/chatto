<!-- @component A quiet loading surface with the same drifting light as the startup screen. Give it an explicit size for its context. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { ClassValue } from 'svelte/elements';
  import { fade } from 'svelte/transition';
  import { expoOutTransition } from './motion';

  let { class: className = '', label = m('common.loading') }: { class?: ClassValue; label?: string } =
    $props();

  function animateFog(node: HTMLElement): () => void {
    let removed = false;
    let stop: (() => void) | undefined;
    // Keep the static gradient while the optional motion code loads.
    void import('./fogGradients')
      .then(({ attachFogGradients }) => {
        if (!removed) stop = attachFogGradients(node);
      })
      .catch(() => undefined);
    return () => {
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
  transition:fade|global={expoOutTransition(120)}
></div>
