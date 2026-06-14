<script lang="ts">
  import type { Snippet } from 'svelte';
  import FieldFootnote from './FieldFootnote.svelte';

  let {
    id,
    checked = $bindable(false),
    label,
    error,
    description,
    disabled = false,
    onchange,
    children
  }: {
    id: string;
    checked?: boolean;
    label?: string;
    error?: string;
    description?: string;
    disabled?: boolean;
    onchange?: (event: Event) => void;
    children?: Snippet;
  } = $props();
</script>

<div class="flex flex-col gap-1">
  <label for={id} class="inline-flex cursor-pointer items-center gap-2">
    <input
      type="checkbox"
      {id}
      bind:checked
      {disabled}
      {onchange}
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

  <FieldFootnote {id} {error} {description} />
</div>
