<!--
@component

Registers global keyboard shortcuts for the application. Include this component
once at the root layout level.

**Shortcuts:**
- `Cmd+Enter` (Mac) / `Ctrl+Enter` (Windows/Linux): Submit the form containing
  the currently focused element
- `Cmd+K` (Mac) / `Ctrl+K` (Windows/Linux): Open the quick switcher palette
-->
<script lang="ts">
  import QuickSwitcher from './QuickSwitcher.svelte';
  import { quickSwitcher } from '$lib/state/globals.svelte';

  function handleKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      quickSwitcher.open();
      return;
    }

    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      const form = document.activeElement?.closest('form');
      if (form) {
        e.preventDefault();
        form.requestSubmit();
      }
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />
<QuickSwitcher />
