<script lang="ts">
  let { canPost = false }: { canPost?: boolean } = $props();
</script>

<button data-testid="thread-composer-send" disabled={!canPost}>Send</button>
