<!--
@component

Test-only wrapper for `CurrentUserBar`. Provides the app UI state that the bar
reads and selects the origin server's first room before the bar mounts.
-->
<script lang="ts">
  import { onMount } from 'svelte';

  import { provideAppUiState, type AppUiState } from '$lib/state/appUi.svelte';
  import CurrentUserBar from './CurrentUserBar.svelte';

  let {
    onReady
  }: {
    onReady?: (state: AppUiState) => void;
  } = $props();

  const appUi = provideAppUiState();
  appUi.setActiveRoomScope('origin', 'room-1');
  onMount(() => onReady?.(appUi));
</script>

<CurrentUserBar />
