<!--
@component

Records whether the last input came from a pointer or a keyboard. Include this
component once at the root layout level.

Pointer input sets `data-input-modality="pointer"` on the root element. A key
press without Ctrl, Meta, or Alt removes it. The `focus-visible` and
`keyboard-modality` variants in `app.css` show focus styles only while the
attribute is absent.

This replaces the browser's own `:focus-visible` heuristic. iOS Safari matches
`:focus-visible` when a menu moves focus by script after a tap or long-press.
Before the first input, and without JavaScript, the attribute is absent and
focus styles follow `:focus-visible`.
-->
<script lang="ts">
  const ATTRIBUTE = 'data-input-modality';

  function handlePointerDown() {
    document.documentElement.setAttribute(ATTRIBUTE, 'pointer');
  }

  function handleKeyDown(e: KeyboardEvent) {
    // Shortcuts such as Cmd+C do not move focus, so they keep pointer mode.
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    document.documentElement.removeAttribute(ATTRIBUTE);
  }

  $effect(() => () => document.documentElement.removeAttribute(ATTRIBUTE));
</script>

<!-- Capture phase: a handler that stops propagation must not hide the input. -->
<svelte:document onpointerdowncapture={handlePointerDown} onkeydowncapture={handleKeyDown} />
