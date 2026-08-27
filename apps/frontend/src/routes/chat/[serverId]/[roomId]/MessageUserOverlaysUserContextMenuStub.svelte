<script lang="ts">
  let { onBanFromRoom }: { onBanFromRoom?: () => void } = $props();
</script>

<button type="button" onclick={() => onBanFromRoom?.()}>Ban from room</button>
