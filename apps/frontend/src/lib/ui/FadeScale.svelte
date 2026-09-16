<!--
@component

Fade and gently zoom content when a containing conditional block opens or closes.
Uses shared compact motion timing and skips animation for reduced motion.
The wrapper preserves its layout space until the exit transition completes.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import { scale } from 'svelte/transition';
  import { cubicInOut } from 'svelte/easing';
  import { COMPACT_MOTION_DURATION_MS, expoOutTransition } from './motion';

  let { children, class: className, id, testId }: {
    children: Snippet;
    class?: ClassValue;
    id?: string;
    testId?: string;
  } = $props();
</script>

<div
  {id}
  class={['min-w-0 origin-bottom-left rtl:origin-bottom-right', className]}
  data-testid={testId}
  in:scale|global={{ ...expoOutTransition(COMPACT_MOTION_DURATION_MS), start: 0.96 }}
  out:scale|global={{ ...expoOutTransition(COMPACT_MOTION_DURATION_MS), easing: cubicInOut, start: 0.96 }}
>
  {@render children()}
</div>
