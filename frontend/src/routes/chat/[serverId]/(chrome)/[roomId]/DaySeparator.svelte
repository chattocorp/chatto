<script lang="ts">
  let { label }: { label: string } = $props();
</script>

<div class="mt-4 flex items-center gap-4 py-3">
  <div class="h-px flex-1 bg-border"></div>
  <span class="text-xs font-medium text-muted/50">{label}</span>
  <div class="h-px flex-1 bg-border"></div>
</div>
