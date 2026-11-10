<!-- @component Supplies app UI context for header integration tests. -->
<script lang="ts">
  import { provideAppUiState } from '$lib/state/appUi.svelte';
  import AppHeader from './AppHeader.svelte';
  import AppCallControls from '$lib/components/voice/AppCallControls.svelte';

  provideAppUiState();
</script>

<AppHeader>
  {#snippet callControls(activeStore)}
    <AppCallControls {activeStore} />
  {/snippet}
</AppHeader>
