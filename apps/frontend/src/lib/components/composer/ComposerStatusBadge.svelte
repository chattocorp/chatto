<!-- @component Shared surface for status badges on the composer input edge. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import { scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  let {
    children,
    avatarPadding = false,
    animate = false,
    class: className,
    ...attributes
  }: HTMLAttributes<HTMLSpanElement> & {
    children: Snippet;
    /** Leave a smaller inset beside typing avatars. */
    avatarPadding?: boolean;
    /** Animate typing activity, respecting reduced motion. */
    animate?: boolean;
  } = $props();
</script>

<span
  {...attributes}
  class={[
    'flex min-h-[26px] max-w-full min-w-0 items-center gap-1.5 rounded-full border border-border bg-background py-0.5 text-xs text-muted shadow-sm',
    avatarPadding ? 'ps-0.5 pe-2' : 'px-2',
    className
  ]}
  transition:scale|global={{
    duration: animate && !userPreferences.reducedMotion ? 150 : 0,
    easing: cubicOut,
    start: 0.96,
    opacity: 0
  }}
>
  {@render children()}
</span>
