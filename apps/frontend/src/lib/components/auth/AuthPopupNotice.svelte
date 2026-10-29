<!-- @component Announces that sign-in continues in a separate window. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  let { pending }: { pending: boolean } = $props();
</script>

{#if pending}
  <p role="status" class="mt-4 text-center text-sm text-muted">{m('auth.popup.waiting')}</p>
{/if}
