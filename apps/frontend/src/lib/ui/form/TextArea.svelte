<script lang="ts">
  import FormField from './FormField.svelte';
  import { fitToBytes } from './byteLimit';

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
    maxBytes,
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
    /** Cut the text at the end so that its UTF-8 encoding fits this many bytes. */
    maxBytes?: number;
    oninput?: (e: Event) => void;
  } = $props();

  const effectiveMaxlength = $derived(maxlength ?? maxBytes);
  function handleInput(event: Event) {
    const field = event.currentTarget as HTMLTextAreaElement;
    const fitted = fitToBytes(field.value, maxBytes ?? Infinity);
    if (fitted !== field.value) field.value = fitted;
    value = fitted;
    oninput?.(event);
  }
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
    maxlength={effectiveMaxlength}
    oninput={maxBytes === undefined ? oninput : handleInput}
    class="input resize-none"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
  ></textarea>
</FormField>
