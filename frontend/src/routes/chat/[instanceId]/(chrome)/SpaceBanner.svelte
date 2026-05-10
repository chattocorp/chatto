<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let { url }: { url: string } = $props();
</script>

<div class="w-full p-2">
  <div class="relative aspect-[1200/630] max-h-32 w-full">
    <div class="absolute inset-0 overflow-hidden rounded-lg bg-surface-100">
      <img
        src={url}
        alt=""
        aria-hidden="true"
        class="pointer-events-none h-full w-full scale-125 object-cover blur-2xl"
      />
    </div>
    <div class="relative flex h-full w-full items-center justify-center">
      <SkeletonImg
        src={url}
        alt="Server banner"
        class="h-full w-auto rounded-lg shadow-lg"
      />
    </div>
  </div>
</div>
