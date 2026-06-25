<!--
@component

Tiny helper text rendered below a form field. Shows the error when one
is present (with role=alert so screen readers announce it on appearance),
otherwise the description. Internal — used by FormField and Checkbox.
-->
<script lang="ts">
  let {
    id,
    error,
    description,
    indent = false
  }: {
    /** Field id; used to derive `${id}-error` / `${id}-description` for aria-describedby. */
    id?: string;
    error?: string;
    description?: string;
    /** Add px-2 when helper text deliberately aligns with input text instead of the field edge. */
    indent?: boolean;
  } = $props();

  const padX = $derived(indent ? 'px-2' : '');
</script>

{#if error}
  <p id={id ? `${id}-error` : undefined} role="alert" class={['text-xs text-error', padX]}>
    {error}
  </p>
{:else if description}
  <p id={id ? `${id}-description` : undefined} class={['text-xs text-muted', padX]}>
    {description}
  </p>
{/if}
