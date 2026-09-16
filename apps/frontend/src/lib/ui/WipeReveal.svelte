<!--
@component

Reveal replacement content from left to right with a soft, diagonal wipe edge.
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
      css: (t: number) => {
        // Move the entire feather beyond either edge at the endpoints. Opposite
        // masks keep outgoing and incoming content on either side of the sweep.
        const edge = -20 + (outgoing ? 1 - t : t) * 140;
        const before = outgoing ? 'transparent' : '#000';
        const after = outgoing ? '#000' : 'transparent';
        return `mask-image: linear-gradient(135deg, ${before} ${edge - 12}%, ${after} ${edge + 12}%);`;
      }
    };
  }
</script>

<div class={['min-w-0', className]} in:wipe|global out:wipe|global={{ outgoing: true }}>
  {@render children()}
</div>
