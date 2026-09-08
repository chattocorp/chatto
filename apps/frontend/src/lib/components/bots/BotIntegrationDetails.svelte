<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    name,
    children,
    status
  }: {
    name: string;
    children: Snippet;
    status?: Snippet;
  } = $props();
</script>

<!-- @component Shared name and metadata layout for bot credentials and outbound endpoints. -->
<div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
  <div class="font-medium text-text-top"><bdi>{name}</bdi></div>
  {@render status?.()}
</div>
<dl class="mt-2 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[max-content_minmax(0,1fr)]">
  {@render children()}
</dl>
