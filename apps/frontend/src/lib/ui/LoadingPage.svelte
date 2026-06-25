<script lang="ts">
  let { message = 'Loading...' }: { message?: string } = $props();
</script>

<div class="flex h-full w-full items-center justify-center">
  <div class="text-lg text-muted">{message}</div>
</div>
