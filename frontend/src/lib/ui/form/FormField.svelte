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
  <label for={id} class="text-sm font-medium text-muted">
    {label}{#if required}<span
        class="ml-1 iconify align-middle text-[0.7em] text-accent uil--asterisk"
        aria-hidden="true"
        title="Required"
      ></span>{/if}
  </label>

  {@render children()}

  <FieldFootnote {id} {error} {description} />
</div>
