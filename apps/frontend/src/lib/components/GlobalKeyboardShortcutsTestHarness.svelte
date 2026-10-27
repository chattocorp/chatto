<script lang="ts">
  import { provideAppUiState } from '$lib/state/appUi.svelte';
  import GlobalKeyboardShortcuts from './GlobalKeyboardShortcuts.svelte';

  provideAppUiState();
</script>

<GlobalKeyboardShortcuts />
<textarea aria-label="Draft"></textarea>
