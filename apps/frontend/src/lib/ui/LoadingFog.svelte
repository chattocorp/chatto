<!-- @component A quiet loading surface with the same drifting light as the startup screen. Give it an explicit size for its context. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { ClassValue } from 'svelte/elements';
  import { fade } from 'svelte/transition';
  import { attachFogGradients } from './fogGradients';
  import { expoOutTransition } from './motion';

  let { class: className = '', label = m('common.loading') }: { class?: ClassValue; label?: string } =
    $props();
</script>

<div
  class={['loading-fog rounded-lg', className]}
  role="status"
  aria-busy="true"
  aria-label={label}
  data-loading-fog
  {@attach attachFogGradients}
  in:fade|global={expoOutTransition(120)}
></div>
