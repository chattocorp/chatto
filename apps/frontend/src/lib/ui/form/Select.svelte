<script lang="ts">
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
    disabled = false
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
  } = $props();
</script>

<FormField {label} {id} {error} {description} {required}>
  <select
    {id}
    bind:value
    {required}
    {disabled}
    class="input"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
  >
    {#if placeholder}
      <option value="" disabled selected={!value}>{placeholder}</option>
    {/if}
    {#each options as option (option.value)}
      <option value={option.value}>{option.label}</option>
    {/each}
  </select>
</FormField>
