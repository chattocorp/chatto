<!--
@component

Full-pane notice for routes the current user may not open: a lock icon,
the "Access Denied" headline, an explanation, and a button back to a safe
destination. Fills its parent and centers the notice in it.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';

  let {
    message = m('ui.access_denied.message'),
    backHref = '/chat',
    backLabel = m('ui.access_denied.back')
  }: {
    /** Explanation shown below the headline. */
    message?: string;
    /** Already-resolved path for the back button. */
    backHref?: string;
    /** Label of the back button. */
    backLabel?: string;
  } = $props();
</script>

<div class="flex h-full w-full flex-col items-center justify-center gap-6 p-6 text-center">
  <div
    class="flex size-16 items-center justify-center rounded-full bg-danger/10 ring-1 ring-danger/20"
  >
    <span aria-hidden="true" class="iconify icon-[uil--lock] text-3xl text-danger"></span>
  </div>
  <div class="flex max-w-sm flex-col gap-2">
    <h1 class="text-2xl font-semibold text-text-top">{m('ui.access_denied.title')}</h1>
    <p class="text-muted">{message}</p>
  </div>
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- backHref is a prop; callers pass already-resolved paths -->
  <a href={backHref} class="btn-secondary">
    <span aria-hidden="true" class="iconify icon-[uil--arrow-left] rtl:-scale-x-100"></span>
    {backLabel}
  </a>
</div>
