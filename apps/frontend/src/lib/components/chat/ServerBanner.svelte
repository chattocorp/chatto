<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { m } from '$lib/i18n/messages';

  let { url }: { url: string } = $props();
</script>

<div class="w-full p-2">
  <div class="media-surface aspect-[1200/630] max-h-32 w-full">
    <SkeletonImg
      src={url}
      alt={m('media.server_banner_alt')}
      class="absolute inset-0 h-full w-full object-cover"
    />
  </div>
</div>
