<!-- @component Consumes an identity-link continuation once, after account data is ready. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { base, resolve } from '$app/paths';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';

  let {
    oncontinue,
    oncomplete
  }: {
    oncontinue: (providerId: string, userId: string) => void;
    oncomplete: (providerId: string, userId: string) => void;
  } = $props();

  onMount(() => {
    const url = new URL(page.url);
    if (!url.searchParams.has('link_provider') && !url.searchParams.has('link_user')) return;
    const providerId = url.searchParams.get('link_provider') ?? '';
    const userId = url.searchParams.get('link_user') ?? '';
    const completed = url.searchParams.get('link_complete') === '1';
    url.searchParams.delete('link_provider');
    url.searchParams.delete('link_user');
    url.searchParams.delete('link_complete');
    replaceState(
      resolve((url.pathname.slice(base.length) + url.search + url.hash) as '/'),
      page.state
    );
    if (completed) oncomplete(providerId, userId);
    else oncontinue(providerId, userId);
  });
</script>
