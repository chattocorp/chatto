<!--
@component

Reusable checkbox option row for forms. Use this for settings, toggles, and
other boolean controls that need a label plus optional helper or error text.
The visible box is custom-styled, while the native checkbox remains in the
DOM for form semantics, keyboard focus, and screen-reader state.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    id,
    checked = $bindable(false),
    label,
    error,
    description,
    disabled = false,
    onchange,
    children
  }: {
    id: string;
    checked?: boolean;
    label?: string;
    error?: string;
    description?: string;
    disabled?: boolean;
    onchange?: (event: Event) => void;
    children?: Snippet;
  } = $props();

  const describedBy = $derived(
    error ? `${id}-error` : description ? `${id}-description` : undefined
  );
</script>

<label
  for={id}
  class={[
    'checkbox-option',
    error && 'border-error/70 bg-error/5 hover:border-error/80 hover:bg-error/10',
    disabled && 'cursor-not-allowed opacity-60 hover:border-border hover:bg-surface/45'
  ]}
>
  <input
    type="checkbox"
    {id}
    bind:checked
    {disabled}
    {onchange}
    class="peer sr-only"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
  />

  <span
    class={[
      'checkbox-box',
      'peer-focus-visible:ring-2 peer-focus-visible:ring-accent/35 peer-focus-visible:ring-offset-0',
      'peer-checked:border-accent peer-checked:bg-accent peer-checked:text-background',
      error && 'border-error peer-focus-visible:ring-error/30'
    ]}
    aria-hidden="true"
  >
    <span class="iconify uil--check text-base"></span>
  </span>

  <span class="flex min-w-0 flex-1 flex-col gap-1">
    <span class="text-sm font-semibold text-text">
      {#if children}
        {@render children()}
      {:else if label}
        {label}
      {/if}
    </span>

    {#if error}
      <span id={`${id}-error`} role="alert" class="text-xs leading-5 text-error">{error}</span>
    {:else if description}
      <span id={`${id}-description`} class="text-xs leading-5 text-muted">{description}</span>
    {/if}
  </span>
</label>
