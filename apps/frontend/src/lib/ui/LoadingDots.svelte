<!--
@component

Three pulsing dots for a pending conversation timeline. Timelines use dots
instead of `LoadingFog` because their content grows from the bottom and has
no stable size to reserve. The dots appear after a short delay, so fast loads
do not flash them. When the user requests less motion, the dots stay still.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { ClassValue } from 'svelte/elements';

  let {
    class: className = '',
    label = m('common.loading')
  }: { class?: ClassValue; label?: string } = $props();
</script>

<div
  class={['flex loading-dots justify-center gap-1.5 text-muted', className]}
  role="status"
  aria-busy="true"
  aria-label={label}
  data-loading-dots
>
  <span class="loading-dot"></span>
  <span class="loading-dot [animation-delay:150ms]"></span>
  <span class="loading-dot [animation-delay:300ms]"></span>
</div>
