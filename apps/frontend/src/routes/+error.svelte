<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { dev } from '$app/environment';
  import * as m from '$lib/i18n/messages';
  import EmptyState from '$lib/ui/EmptyState.svelte';

  const status = $derived(page.status);
  const message = $derived(page.error?.message ?? m['error_page.unknown']());
</script>

<div class="flex h-full flex-col">
  <EmptyState icon="uil--exclamation-triangle" title={m['error_page.title']()}>
    <p>{m['error_page.description']()}</p>
    {#if dev}
      <pre
        class="mt-3 max-w-xl overflow-auto rounded-md bg-surface-200 p-3 text-left text-xs">Status: {status}
{message}</pre>
    {/if}
    <p class="mt-4">
      <a class="underline" href={resolve('/')}>{m['error_page.home_link']()}</a>
    </p>
  </EmptyState>
</div>
