<script lang="ts">
  import FormField from './FormField.svelte';
  import { byteLimitDescription, createByteLimit } from './byteLimit';

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
    /** Reject edits whose UTF-8 encoding would exceed this many bytes. */
    maxBytes?: number;
    oninput?: (e: Event) => void;
  } = $props();

  const effectiveDescription = $derived(byteLimitDescription(description, maxBytes, value));
  const effectiveMaxlength = $derived(maxlength ?? maxBytes);
  const byteLimit = createByteLimit(() => maxBytes ?? Infinity);

  $effect(() => {
    byteLimit.track(value);
  });

  function handleInput(event: Event) {
    value = byteLimit.accept(event.currentTarget as HTMLTextAreaElement);
    oninput?.(event);
  }
</script>

<FormField {label} {id} {error} description={effectiveDescription} {required}>
  <textarea
    {id}
    data-testid={testid}
    onbeforeinput={maxBytes === undefined ? undefined : byteLimit.beforeinput}
    bind:value
    {placeholder}
    {required}
    {disabled}
    {rows}
    maxlength={effectiveMaxlength}
    oninput={maxBytes === undefined ? oninput : handleInput}
    class="input resize-none"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error
      ? `${id}-error`
      : effectiveDescription
        ? `${id}-description`
        : undefined}
  ></textarea>
</FormField>
