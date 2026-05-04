<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let { url }: { url: string } = $props();
</script>

<div class="w-64 p-2">
  <SkeletonImg
    src={url}
    alt="Space banner"
    class="aspect-4/3 w-full rounded-lg object-cover shadow-md"
  />
</div>
