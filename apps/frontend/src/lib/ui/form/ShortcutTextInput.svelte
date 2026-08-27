<!--
@component

A `TextInput` that claims a Command/Control shortcut, advertises the matching
platform hint in its placeholder, and selects the existing value when focused
by that shortcut. Use for high-frequency filters and search fields where
typing should immediately replace the current query.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { usesAppleShortcutModifier } from '$lib/utils/platformShortcuts';
  import TextInput from './TextInput.svelte';

  let {
    shortcutKey,
    label,
    id,
    testid,
    value = $bindable(''),
    placeholder,
    error,
    description,
    required = false,
    labelHidden = false,
    disabled = false,
    autocomplete,
    minlength,
    maxlength,
    leadingIcon,
    trailingText,
    onkeydown,
    oninput
  }: {
    /** The key pressed with Command on Apple platforms or Control elsewhere. */
    shortcutKey: string;
    label: string;
    id: string;
    testid?: string;
    value?: string;
    placeholder?: string;
    error?: string;
    description?: string;
    required?: boolean;
    labelHidden?: boolean;
    disabled?: boolean;
    autocomplete?: HTMLInputAttributes['autocomplete'];
    minlength?: number;
    maxlength?: number;
    leadingIcon?: string;
    trailingText?: string;
    onkeydown?: (event: KeyboardEvent) => void;
    oninput?: (event: Event) => void;
  } = $props();

  let shortcutModifier = $state<string | null>(null);
  const shortcutHint = $derived(shortcutModifier ? `${shortcutModifier}${shortcutKey}` : null);
  const placeholderWithShortcut = $derived(
    `${placeholder ?? ''}${shortcutHint ? `${placeholder ? ' ' : ''}(${shortcutHint})` : ''}`
  );

  onMount(() => {
    shortcutModifier = usesAppleShortcutModifier() ? '⌘' : 'Ctrl-';
  });

  function inputForShortcut(): HTMLInputElement | null {
    const element = document.getElementById(id);
    return element instanceof HTMLInputElement ? element : null;
  }

  function handleWindowKeydown(event: KeyboardEvent) {
    const inputElement = inputForShortcut();
    if (
      event.defaultPrevented ||
      event.altKey ||
      event.shiftKey ||
      (!event.metaKey && !event.ctrlKey) ||
      event.key.toLowerCase() !== shortcutKey.toLowerCase() ||
      !inputElement ||
      inputElement.disabled
    ) {
      return;
    }

    event.preventDefault();
    inputElement.focus();
    inputElement.select();
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<TextInput
  {label}
  {id}
  {testid}
  bind:value
  {error}
  {description}
  {required}
  {labelHidden}
  {disabled}
  {autocomplete}
  {minlength}
  {maxlength}
  {leadingIcon}
  {trailingText}
  {onkeydown}
  {oninput}
  placeholder={placeholderWithShortcut}
/>
