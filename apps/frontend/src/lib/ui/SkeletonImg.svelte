<script lang="ts">
  import type { HTMLImgAttributes } from 'svelte/elements';

  let {
    class: className,
    onload,
    onerror,
    ...rest
  }: HTMLImgAttributes & {
    onload?: (event: Event) => void;
    onerror?: (event: Event) => void;
  } = $props();
  let loaded = $state(false);
</script>

<img
  class={[className, !loaded && 'skeleton']}
  onload={(event) => {
    loaded = true;
    onload?.(event);
  }}
  onerror={(event) => {
    loaded = false;
    onerror?.(event);
  }}
  {...rest}
/>
