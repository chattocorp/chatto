<!--
@component

Drives `idleState.isInputFocused` from document focus events. Mount once
in the root layout. The rest of the heuristics for "is the user idle"
live in `idleState` (see `$lib/state/idle.svelte`).
-->
<script lang="ts">
  import { idleState, isEditableElement } from '$lib/state/idle.svelte';

  function syncFocus() {
    idleState.isInputFocused = isEditableElement(document.activeElement);
  }
</script>

<svelte:document onfocusin={syncFocus} onfocusout={syncFocus} />
