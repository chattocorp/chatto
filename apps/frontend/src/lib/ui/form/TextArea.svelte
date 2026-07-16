<script lang="ts">
  import * as m from '$lib/i18n/messages';
  import FormField from './FormField.svelte';

  const textEncoder = new TextEncoder();

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

  const effectiveDescription = $derived(
    maxBytes === undefined
      ? description
      : [description, m['ui.form.max_bytes']({ max: maxBytes })].filter(Boolean).join(' ')
  );
  const effectiveMaxlength = $derived(maxlength ?? maxBytes);

  function handleBeforeInput(event: InputEvent) {
    if (maxBytes === undefined || !event.inputType.startsWith('insert')) return;

    const input = event.currentTarget as HTMLTextAreaElement;
    const insertedText =
      event.data ??
      event.dataTransfer?.getData('text/plain') ??
      (event.inputType === 'insertLineBreak' || event.inputType === 'insertParagraph'
        ? '\n'
        : null);
    if (insertedText == null) return;

    const start = input.selectionStart ?? input.value.length;
    const end = input.selectionEnd ?? start;
    const nextValue = input.value.slice(0, start) + insertedText + input.value.slice(end);
    if (textEncoder.encode(nextValue).byteLength > maxBytes) event.preventDefault();
  }
</script>

<FormField {label} {id} {error} description={effectiveDescription} {required}>
  <textarea
    {id}
    data-testid={testid}
    onbeforeinput={handleBeforeInput}
    bind:value
    {placeholder}
    {required}
    {disabled}
    {rows}
    maxlength={effectiveMaxlength}
    {oninput}
    class="input resize-none"
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error
      ? `${id}-error`
      : effectiveDescription
        ? `${id}-description`
        : undefined}
  ></textarea>
</FormField>
