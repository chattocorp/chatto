<!--
@component

Test-only wrapper for `CurrentUserBar`. Provides the app UI state that the bar
reads and selects the origin server's first room before the bar mounts.
-->
<script lang="ts">
  import { onMount } from 'svelte';

  import { provideAppUiState, type AppUiState } from '$lib/state/appUi.svelte';
  import CurrentUserBar from './CurrentUserBar.svelte';
  import AppCallControls from './voice/AppCallControls.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  let {
    onReady,
    callControls = false
  }: {
    onReady?: (state: AppUiState) => void;
    callControls?: boolean;
  } = $props();

  const appUi = provideAppUiState();
  const scope = useServerScope();
  appUi.setActiveRoomScope('origin', 'room-1');
  onMount(() => onReady?.(appUi));
</script>

{#if callControls}
  <AppCallControls activeStore={scope.store} />
{:else}
  <CurrentUserBar />
{/if}
