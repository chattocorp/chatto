<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import * as m from '$lib/i18n/messages';

  let { url }: { url: string } = $props();
</script>

<div class="w-full p-2">
  <div class="relative aspect-[1200/630] max-h-32 w-full overflow-hidden rounded-lg bg-surface">
    <SkeletonImg
      src={url}
      alt={m['media.server_banner_alt']()}
      class="absolute inset-0 h-full w-full rounded-lg object-cover shadow-lg"
    />
  </div>
</div>
