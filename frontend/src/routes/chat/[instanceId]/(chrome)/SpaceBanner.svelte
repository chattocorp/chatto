<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let { url }: { url: string } = $props();
</script>

<div class="w-64 p-2">
  <SkeletonImg
    src={url}
    alt="Server banner"
    class="aspect-[1200/630] w-full rounded-lg object-cover shadow-md"
  />
</div>
