<script lang="ts">
  import type { Snippet } from 'svelte';

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
    {label}{#if required}<span class="ml-0.5 text-error">*</span>{/if}
  </label>

  {@render children()}

  {#if error}
    <p id="{id}-error" class="text-xs text-error">{error}</p>
  {:else if description}
    <p id="{id}-description" class="text-xs text-muted">{description}</p>
  {/if}
</div>
