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
    gap: 12px;
    padding: 12px 18px 6px;
    color: var(--color-muted);
    font-size: 11px;
  }
  .line {
    flex-grow: 1;
    height: 1px;
    background-color: var(--color-border);
  }
</style>
