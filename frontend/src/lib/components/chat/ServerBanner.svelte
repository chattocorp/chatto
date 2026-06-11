<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let { url }: { url: string } = $props();
</script>

<div class="w-full p-2">
  <div class="relative aspect-[1200/630] max-h-32 w-full overflow-hidden rounded-lg bg-surface-100">
    <div
      aria-hidden="true"
      class="pointer-events-none absolute inset-y-0 left-1/2 flex -translate-x-1/2 items-center blur-lg"
    >
      <img src={url} alt="" class="h-full w-auto max-w-none shrink-0 -scale-x-100" />
      <img src={url} alt="" class="h-full w-auto max-w-none shrink-0" />
      <img src={url} alt="" class="h-full w-auto max-w-none shrink-0 -scale-x-100" />
    </div>
    <SkeletonImg
      src={url}
      alt="Server banner"
      class="absolute inset-0 h-full w-full rounded-lg object-contain shadow-lg"
    />
  </div>
</div>
