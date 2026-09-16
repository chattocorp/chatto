<script lang="ts">
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { m } from '$lib/i18n/messages';

  let { url }: { url: string } = $props();
</script>

<!-- @component Full-width server artwork separated from the sidebar navigation. -->
<div class="relative aspect-[1200/630] max-h-32 w-full overflow-hidden border-b border-border bg-surface">
  <SkeletonImg
    src={url}
    alt={m('media.server_banner_alt')}
    class="absolute inset-0 h-full w-full object-cover"
  />
</div>
