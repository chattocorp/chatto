<script lang="ts">
  import { m } from '$lib/i18n/messages';

  let { url }: { url: string } = $props();
</script>

<!-- @component Full-width server artwork separated from the sidebar navigation. -->
<div class="w-full border-b border-border bg-surface">
  <img
    src={url}
    alt={m('media.server_banner_alt')}
    class="block h-auto w-full"
onload={(event) => ((event.currentTarget as HTMLImageElement).style.display = '')}
onerror={(event) => ((event.currentTarget as HTMLImageElement).style.display = 'none')}
  />
</div>
