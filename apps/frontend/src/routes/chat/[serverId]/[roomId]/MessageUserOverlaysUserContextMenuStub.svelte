<script lang="ts">
  let { onBanFromRoom }: { onBanFromRoom?: () => void } = $props();
</script>

<button type="button" onclick={() => onBanFromRoom?.()}>Remove from room</button>
