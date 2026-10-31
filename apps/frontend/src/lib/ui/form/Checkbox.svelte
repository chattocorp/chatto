<!--
@component

Reusable checkbox option row for forms. Use this for settings, toggles, and
other boolean controls that need a label plus optional helper or error text.
The complete row shows the selected state, and its square indicator distinguishes
it from the circular ChoiceRow radio pattern. The row positions the hidden native input so focus stays inside scrolling panes.
The native checkbox remains in
the DOM for form semantics, keyboard focus, and screen-reader state.
-->
<script lang="ts">
  import { tick, type Snippet } from 'svelte';

  let {
    id,
    checked = $bindable(false),
    label,
    error,
    description,
    disabled = false,
    loading = false,
    onchange,
    onCheckedChange,
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
    /** Owns the committed value and failure feedback for an asynchronous save. */
    onCheckedChange?: (checked: boolean) => void | Promise<void>;
    children?: Snippet;
  } = $props();

  let pending = $state(false);

  async function change(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    if (!onCheckedChange) return;
    const next = input.checked;
    input.checked = checked;
    if (pending) return;
    const restoreFocus = input === input.ownerDocument.activeElement;
    pending = true;
    try {
      await onCheckedChange(next);
    } catch {
      // The owner reports failure; keep its last committed value.
    } finally {
      await tick();
      input.checked = checked;
      pending = false;
      await tick();
      if (
        restoreFocus &&
        input.isConnected &&
        !input.disabled &&
        input.ownerDocument.activeElement === input.ownerDocument.body
      )
        input.focus({ preventScroll: true });
    }
  }

  const describedBy = $derived(
    error ? `${id}-error` : description ? `${id}-description` : undefined
  );
</script>

<label
  for={id}
  aria-busy={loading || pending || undefined}
  class={[
    'relative checkbox-option',
    checked && !error && 'checkbox-option-selected',
    error && 'checkbox-option-error',
    disabled && 'cursor-not-allowed opacity-60',
    disabled && !checked && !error && 'hover:border-border hover:bg-transparent'
  ]}
>
  {#if onCheckedChange}
    <input
      type="checkbox"
      {id}
      {checked}
      disabled={disabled || loading || pending}
      onchange={change}
      class="peer sr-only"
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={describedBy}
    />
  {:else}
    <!-- Preserve Svelte's ordinary binding and native form-reset behavior. -->
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
  {/if}

  <span
    class={[
      'checkbox-box',
      'peer-focus-visible:ring-2 peer-focus-visible:ring-action/35 peer-focus-visible:ring-offset-0',
      checked && 'checkbox-box-selected',
      error && 'checkbox-box-error peer-focus-visible:ring-error/30',
      (loading || pending) && 'animate-pulse'
    ]}
    aria-hidden="true"
  >
    <span aria-hidden="true" class="iconify icon-[uil--check] text-base"></span>
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
