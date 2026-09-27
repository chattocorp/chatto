<script lang="ts">
  import { onMount } from 'svelte';

  let {
    canPost = false,
    onReady
  }: { canPost?: boolean; onReady?: (api: { focus: () => void; addFiles: () => void }) => void } =
    $props();

  onMount(() => onReady?.({ focus: () => {}, addFiles: () => {} }));
</script>

<button data-testid="thread-composer-send" disabled={!canPost}>Send</button>
