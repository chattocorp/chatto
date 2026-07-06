<script lang="ts">
  let { label = '' }: { label?: string } = $props();
</script>

{#if label}
  <div class="my-6 flex items-center gap-3 text-sm text-muted">
    <div class="h-px flex-1 bg-border"></div>
    {label}
    <div class="h-px flex-1 bg-border"></div>
  </div>
{:else}
  <div class="my-6 h-px bg-border"></div>
{/if}
