<!-- @component A loading surface for pending media, such as images and video, with a soft highlight that sweeps across it. Give it an explicit size for its context. Use `LoadingDots` for other pending content. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { ClassValue } from 'svelte/elements';
  import { fade } from 'svelte/transition';
  import { expoOutTransition } from './motion';

  let {
    class: className = '',
    label = m('common.loading')
  }: { class?: ClassValue; label?: string } = $props();
</script>

<div
  class={['loading-fog rounded-lg', className]}
  role="status"
  aria-busy="true"
  aria-label={label}
  data-loading-fog
  in:fade|global={expoOutTransition(120)}
></div>
