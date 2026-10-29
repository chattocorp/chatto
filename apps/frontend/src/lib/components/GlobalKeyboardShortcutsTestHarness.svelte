<script lang="ts">
  import { provideAppUiState } from '$lib/state/appUi.svelte';
  import GlobalKeyboardShortcuts from './GlobalKeyboardShortcuts.svelte';
  import { provideKeyboardShortcuts } from '$lib/keyboardShortcuts';
  import { ShortcutTextInput } from '$lib/ui/form';
  import ComposerShortcutHints from './composer/ComposerShortcutHints.svelte';

  let { showFilter = false, showEditor = false }: { showFilter?: boolean; showEditor?: boolean } =
    $props();

  provideAppUiState();
  provideKeyboardShortcuts();
</script>

<GlobalKeyboardShortcuts />
<textarea aria-label="Draft"></textarea>
{#if showFilter}<ShortcutTextInput
    id="help-filter"
    label="Filter permissions"
    shortcutKey="/"
  />{/if}
{#if showEditor}<ComposerShortcutHints />{/if}
