<script lang="ts">
  let { onConfirm }: { onConfirm: () => void } = $props();
</script>

<button data-testid="confirm-role-delete" onclick={onConfirm}>Confirm deletion</button>
