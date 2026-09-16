<!--
@component

A joined group of independent actions. Use pill-button on each native button.
The regular size aligns with shell inputs; compact is for secondary control rows.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';

  let {
    label,
    compact = false,
    gaps = true,
    class: className,
    testId,
    children
  }: {
    label: string;
    compact?: boolean;
    /** Separate 3D segments with gaps and rounded inner corners. */
    gaps?: boolean;
    class?: ClassValue;
    testId?: string;
    children: Snippet;
  } = $props();
</script>

<div
  role="group"
  aria-label={label}
  class={['pill-button-group', compact && 'pill-button-group-compact', className]}
  data-gaps={gaps}
  data-testid={testId}
>
  {@render children()}
</div>
