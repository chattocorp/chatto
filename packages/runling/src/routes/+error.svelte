<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
</script>

<!-- @component Render console errors from the console's source tree, so Vite does not
resolve SvelteKit's external fallback component relative to the workflow project's cwd. -->
<svelte:head>
  <title>{page.status} — Runling</title>
</svelte:head>

<main class="flex min-h-screen flex-col items-center justify-center gap-4 p-8">
  <h1 class="text-2xl font-semibold">
    {page.status === 404 ? 'Page not found' : 'Page unavailable'}
  </h1>
  <p>Return to Runling to view your workflows.</p>
  <a class="underline" href={resolve('/')}>Open Runling</a>
</main>
