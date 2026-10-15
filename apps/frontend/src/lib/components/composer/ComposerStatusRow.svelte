<!-- @component Positions status badges on the input edge without changing layout. The parent must match the input surface and use relative positioning. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  let { children }: { children: Snippet } = $props();
</script>

<div
  class="pointer-events-none absolute inset-x-3 bottom-full z-10 mb-1 flex min-w-0 translate-y-1/2 items-center justify-end gap-1.5"
>
  {@render children()}
</div>
