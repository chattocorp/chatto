<!-- @component A native single-choice field with a themed picker where supported. -->
<script lang="ts">
  import { tick } from 'svelte';
  import FormField from './FormField.svelte';

  type Option = {
    value: string;
    label: string;
  };

  let {
    label,
    id,
    value = $bindable(''),
    options,
    placeholder,
    error,
    description,
    required = false,
    disabled = false,
    onValueChange
  }: {
    label: string;
    id: string;
    value?: string;
    options: Option[];
    placeholder?: string;
    error?: string;
    description?: string;
    required?: boolean;
    disabled?: boolean;
    /** Owns the committed value and failure feedback. When set, value is not assigned locally. */
    onValueChange?: (value: string) => void | Promise<void>;
  } = $props();

  let pending = $state(false);
  // Render the label ourselves. Native selectedcontent cloning mutates this
  // subtree before change, which makes Svelte's select observer restore the old value.
  const selectedLabel = $derived(
    options.find((option) => option.value === value)?.label ?? placeholder ?? ''
  );

  async function change(event: Event) {
    const element = event.currentTarget as HTMLSelectElement;
    const next = element.value;
    if (pending) {
      element.value = value;
      return;
    }
    if (!onValueChange) {
      value = next;
      return;
    }

    const restoreFocus = element.contains(element.ownerDocument.activeElement);
    pending = true;
    // Keep the committed selection visible until the owner applies the change.
    element.value = value;
    try {
      await onValueChange(next);
    } catch {
      // The owner reports failures; rejection must not commit the requested value.
    } finally {
      await tick();
      element.value = value;
      pending = false;
      await tick();
      // Disabling a pending control can move focus to the body. Do not steal focus
      // if the user moved to another control while the operation was pending.
      if (
        restoreFocus &&
        element.isConnected &&
        !element.disabled &&
        element.ownerDocument.activeElement === element.ownerDocument.body
      ) {
        element.focus({ preventScroll: true });
      }
    }
  }
</script>

<FormField {label} {id} {error} {description} {required}>
  <select
    {id}
    {value}
    onchange={change}
    {required}
    disabled={disabled || pending}
    aria-busy={pending || undefined}
    class="select-control"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
  >
    <button type="button" class="flex min-w-0 flex-1 items-center text-start">
      <span class="block min-w-0 truncate">{selectedLabel}</span>
    </button>
    {#if placeholder}
      <option value="" disabled selected={!value}>{placeholder}</option>
    {/if}
    {#each options as option (option.value)}
      <option value={option.value}>{option.label}</option>
    {/each}
  </select>
</FormField>
