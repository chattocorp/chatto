<!--
@component

Reveal replacement content from left to right with a soft, diagonal wipe edge.
Change `active` to replace the content in one grid cell. Mounting or removing
the whole component does not animate. Reduced motion skips the wipe.
-->
<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import { cubicInOut } from 'svelte/easing';
  import { expoOutTransition } from './motion';

  let {
    active,
    children,
    class: className
  }: {
    active: boolean;
    children: Snippet<[boolean]>;
    class?: ClassValue;
  } = $props();
  let mounted = false;
  onMount(() => {
    mounted = true;
  });

  function wipe(_node: Element, { outgoing = false } = {}) {
    return {
      ...expoOutTransition(mounted ? 320 : 0),
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

<div class={['grid min-w-0 items-end', className]}>
  {#key active}
    <div class="col-start-1 row-start-1 min-w-0" in:wipe out:wipe={{ outgoing: true }}>
      {@render children(active)}
    </div>
  {/key}
</div>
