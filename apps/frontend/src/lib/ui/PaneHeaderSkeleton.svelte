<script lang="ts">
  let { buttons = 3 }: { buttons?: number } = $props();
</script>

<div class="flex w-full flex-row gap-2">
  <div class="skeleton h-5 flex-1 rounded-md"></div>
  {#each Array(buttons) as _, i (i)}
    <div class="skeleton h-5 w-5 rounded-md"></div>
  {/each}
</div>
