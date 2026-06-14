<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let { url }: { url: string } = $props();
</script>

<div class="w-full p-2">
  <div class="relative aspect-[1200/630] max-h-32 w-full overflow-hidden rounded-lg bg-surface-100">
    <SkeletonImg
      src={url}
      alt="Server banner"
      class="absolute inset-0 h-full w-full rounded-lg object-cover shadow-lg"
    />
  </div>
</div>
