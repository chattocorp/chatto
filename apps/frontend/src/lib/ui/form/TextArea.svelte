<script lang="ts">
  import FormField from './FormField.svelte';

  let {
    label,
    id,
    testid,
    value = $bindable(''),
    placeholder,
    error,
    description,
    required = false,
    disabled = false,
    rows = 3,
    maxlength,
    oninput
  }: {
    label: string;
    id: string;
    testid?: string;
    value?: string;
    placeholder?: string;
    error?: string;
    description?: string;
    required?: boolean;
    disabled?: boolean;
    rows?: number;
    maxlength?: number;
    oninput?: (e: Event) => void;
  } = $props();
</script>

<FormField {label} {id} {error} {description} {required}>
  <textarea
    {id}
    data-testid={testid}
    bind:value
    {placeholder}
    {required}
    {disabled}
    {rows}
    {maxlength}
    {oninput}
    class="input resize-none"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
  ></textarea>
</FormField>
