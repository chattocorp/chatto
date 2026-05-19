<script lang="ts">
  import type { Snippet } from 'svelte';
  import FieldFootnote from './FieldFootnote.svelte';

  let {
    label,
    id,
    error,
    description,
    required = false,
    children
  }: {
    label: string;
    id?: string;
    error?: string;
    description?: string;
    required?: boolean;
    children: Snippet;
  } = $props();
</script>

<div class="flex flex-col gap-1.5">
  <!--
    Labels and helper text are indented by the input's interior padding
    so the label baseline aligns with the input's value text.
  -->
  <label for={id} class="px-2 text-sm font-medium text-muted">
    {label}{#if required}<span
        class="iconify uil--asterisk ml-1 align-middle text-[0.7em] text-accent"
        aria-hidden="true"
        title="Required"
      ></span>{/if}
  </label>

  {@render children()}

  <FieldFootnote {id} {error} {description} indent />
</div>
