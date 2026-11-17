<!--
@component

A small animated braille spinner for work that is in progress, such as a
running background job. It is decorative: put it next to text that names the
state. When Chatto's motion setting is reduced, it shows one still frame.

```svelte
<span class="inline-flex items-center gap-1.5"><ActivityIndicator /> Running</span>
```
-->
<script lang="ts">
  import type { ClassValue } from 'svelte/elements';

  let { class: className = '' }: { class?: ClassValue } = $props();

  const frames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];
</script>

<span
  class={[
    'inline-block h-[1em] w-[1ch] shrink-0 overflow-hidden font-mono leading-none',
    className
  ]}
  aria-hidden="true"
  data-activity-indicator
>
  <span class="block braille-spinner">
    {#each frames as frame (frame)}
      <span class="block h-[1em]">{frame}</span>
    {/each}
  </span>
</span>
