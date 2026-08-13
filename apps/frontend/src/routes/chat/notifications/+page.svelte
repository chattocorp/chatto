<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { notificationCenter } from '$lib/state/globals.svelte';
  import { onMount } from 'svelte';

  onMount(() => {
    notificationCenter.open();
    void goto(resolve('/chat'), { replaceState: true });
  });
</script>

<!-- Compatibility bridge for old notification-page links. -->
