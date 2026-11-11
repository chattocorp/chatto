<!--
@component
Announces that sign-in continues in a separate window. `oncancel` adds a
button that stops waiting: a provider page can hide that the user closed the
window.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { Button } from '$lib/ui/form';
  let { pending, oncancel }: { pending: boolean; oncancel?: () => void } = $props();
</script>

{#if pending}
  <div class="mt-4 flex flex-col items-center gap-2">
    <p role="status" class="text-center text-sm text-muted">{m('auth.popup.waiting')}</p>
    {#if oncancel}
      <Button variant="ghost" size="sm" onclick={oncancel}>{m('common.cancel')}</Button>
    {/if}
  </div>
{/if}
