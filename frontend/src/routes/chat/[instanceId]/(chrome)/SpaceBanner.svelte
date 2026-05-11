<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let { url }: { url: string } = $props();
</script>

<div class="w-full p-2">
  <div class="relative h-32 w-full overflow-hidden rounded-lg bg-surface-100">
    <img
      src={url}
      alt=""
      aria-hidden="true"
      class="pointer-events-none absolute inset-0 h-full w-full scale-125 object-cover blur-2xl"
    />
    <SkeletonImg
      src={url}
      alt="Server banner"
      class="absolute inset-0 h-full w-full rounded-lg object-contain shadow-lg"
    />
  </div>
</div>
