<!--
@component

Reveal replacement content from left to right with a straight wipe edge.
Place outgoing and incoming wrappers in the same grid cell. The outgoing
content clears behind the incoming edge. Reduced motion skips the wipe.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import { cubicInOut } from 'svelte/easing';
  import { expoOutTransition } from './motion';

  let { children, class: className }: { children: Snippet; class?: ClassValue } = $props();

  function wipe(_node: Element, { outgoing = false } = {}) {
    return {
      ...expoOutTransition(320),
      easing: cubicInOut,
      css: (t: number) => outgoing
        ? `clip-path: inset(0 0 0 ${(1 - t) * 100}%);`
        : `clip-path: inset(0 ${(1 - t) * 100}% 0 0);`
    };
  }
</script>

<div class={['min-w-0', className]} in:wipe|global out:wipe|global={{ outgoing: true }}>
  {@render children()}
</div>
