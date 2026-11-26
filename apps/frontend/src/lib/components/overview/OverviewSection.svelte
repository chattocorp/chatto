<!--
@component

A titled section of the server Overview. The small uppercase title groups
several cards without adding another frame around them.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, children }: { title: string; children: Snippet } = $props();

  const titleId = $props.id();
</script>

<section class="flex flex-col gap-3" aria-labelledby={titleId}>
  <h2 id={titleId} class="px-1 section-label text-muted">
    {title}
  </h2>
  {@render children()}
</section>
