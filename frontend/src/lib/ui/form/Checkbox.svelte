<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    id,
    checked = $bindable(false),
    label,
    error,
    description,
    disabled = false,
    children
  }: {
    id: string;
    checked?: boolean;
    label?: string;
    error?: string;
    description?: string;
    disabled?: boolean;
    children?: Snippet;
  } = $props();
</script>

<div>
  <label for={id} class="inline-flex cursor-pointer items-center gap-2">
    <input
      type="checkbox"
      {id}
      bind:checked
      {disabled}
      class="checkbox"
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
    />
    <span class="text-sm">
      {#if children}
        {@render children()}
      {:else if label}
        {label}
      {/if}
    </span>
  </label>

  {#if error}
    <p id="{id}-error" role="alert" class="mt-1 text-xs text-error">{error}</p>
  {:else if description}
    <p id="{id}-description" class="mt-1 text-xs text-muted">{description}</p>
  {/if}
</div>
