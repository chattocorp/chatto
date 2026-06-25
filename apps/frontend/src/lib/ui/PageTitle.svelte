<script lang="ts">
  import { titleState } from '$lib/state/globals.svelte';

  let { title }: { title: string } = $props();

  $effect(() => {
    titleState.setPageTitle(title);
    return () => titleState.clearPageTitle();
  });
</script>
