<!--
@component

Reusable checkbox option row for forms. Use this for settings, toggles, and
other boolean controls that need a label plus optional helper or error text.
The complete row shows the selected state, and its square indicator distinguishes
it from the circular ChoiceRow radio pattern. The native checkbox remains in
the DOM for form semantics, keyboard focus, and screen-reader state.
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
    loading = false,
    onchange,
    children
  }: {
    id: string;
    checked?: boolean;
    label?: string;
    error?: string;
    description?: string;
    disabled?: boolean;
    loading?: boolean;
    onchange?: (event: Event) => void;
    children?: Snippet;
  } = $props();

  const describedBy = $derived(
    error ? `${id}-error` : description ? `${id}-description` : undefined
  );
</script>

<label
  for={id}
  aria-busy={loading || undefined}
  class={[
    'checkbox-option',
    checked && !error && 'checkbox-option-selected',
    error && 'checkbox-option-error',
    disabled && 'cursor-not-allowed opacity-60',
    disabled && !checked && !error && 'hover:border-border hover:bg-transparent'
  ]}
>
  <input
    type="checkbox"
    {id}
    bind:checked
    disabled={disabled || loading}
    {onchange}
    class="peer sr-only"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
  />

  <span
    class={[
      'checkbox-box',
      'peer-focus-visible:ring-2 peer-focus-visible:ring-action/35 peer-focus-visible:ring-offset-0',
      checked && 'checkbox-box-selected',
      error && 'checkbox-box-error peer-focus-visible:ring-error/30',
      loading && 'animate-pulse'
    ]}
    aria-hidden="true"
  >
    <span class="iconify text-base icon-[uil--check]"></span>
  </span>

  <span class="min-w-0 flex-1">
    <span class={['block text-text', checked && 'font-medium']}>
      {#if children}
        {@render children()}
      {:else if label}
        {label}
      {/if}
    </span>

    {#if error}
      <span id={`${id}-error`} role="alert" class="block text-sm text-error">{error}</span>
    {:else if description}
      <span id={`${id}-description`} class="block text-sm text-muted">{description}</span>
    {/if}
  </span>
</label>
