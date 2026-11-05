<script lang="ts">
  let { label }: { label: string } = $props();
</script>

<div class="divider">
  <div class="line"></div>
  <text>{label}</text>
  <div class="line"></div>
</div>

<style>
  .divider {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-12);
    padding: var(--size-12) var(--size-18) var(--size-6);
    color: var(--color-muted);
    font-size: var(--text-small);
  }
  .line {
    flex-grow: 1;
    height: var(--size-1);
    background-color: var(--color-border);
  }
</style>
