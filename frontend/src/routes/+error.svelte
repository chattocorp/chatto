<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { dev } from '$app/environment';
  import { EmptyState } from '$lib/ui';

  const status = $derived(page.status);
  const message = $derived(page.error?.message ?? 'Unknown error');
</script>

<div class="flex h-full flex-col">
  <EmptyState icon="uil--exclamation-triangle" title="Something went wrong">
    <p>An unexpected error occurred while rendering this page.</p>
    {#if dev}
      <pre
        class="mt-3 max-w-xl overflow-auto rounded-md bg-surface-200 p-3 text-left text-xs"
      >Status: {status}
{message}</pre>
    {/if}
    <p class="mt-4">
      <a class="underline" href={resolve('/')}>Go back to the home page</a>
    </p>
  </EmptyState>
</div>
