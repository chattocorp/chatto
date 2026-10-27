<!--
@component

Registers one window keyboard shortcut while mounted. Modifiers must match
exactly; `command` accepts either Cmd or Ctrl. Capture runs before editor
bindings. Set `capture={false}` for a fallback that local handlers can cancel.
Matched repeats are consumed without running the action again.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getKeyboardShortcuts } from '$lib/keyboardShortcuts';

  let {
    key,
    description,
    command = false,
    alt = false,
    shift = false,
    capture = true,
    allowInModal = true,
    when = () => true,
    ontrigger
  }: {
    /** KeyboardEvent.key, compared without case. */
    key: string;
    /** Localised help label. Omit for bindings that should not appear in app help. */
    description?: string;
    command?: boolean;
    alt?: boolean;
    shift?: boolean;
    capture?: boolean;
    allowInModal?: boolean;
    /** Checked at keydown time, before the event is consumed. */
    when?: () => boolean;
    ontrigger: () => void;
  } = $props();

  const registry = getKeyboardShortcuts();
  // The registration is a component-lifetime resource; its getter reads current props.
  onMount(() =>
    registry?.register(() =>
      description && eligible()
        ? [{ label: description, key, command, alt, shift, group: 'app' }]
        : []
    )
  );

  function eligible() {
    return (allowInModal || !document.querySelector('dialog[open], [aria-modal="true"]')) && when();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || event.isComposing) return;
    const commandMatches = command
      ? event.metaKey !== event.ctrlKey
      : !event.metaKey && !event.ctrlKey;
    if (
      !commandMatches ||
      event.altKey !== alt ||
      event.shiftKey !== shift ||
      event.key.toLowerCase() !== key.toLowerCase()
    )
      return;
    if (!eligible()) return;

    event.preventDefault();
    event.stopPropagation();
    if (!event.repeat) ontrigger();
  }
</script>

<svelte:window
  onkeydowncapture={capture ? handleKeydown : undefined}
  onkeydown={capture ? undefined : handleKeydown}
/>
