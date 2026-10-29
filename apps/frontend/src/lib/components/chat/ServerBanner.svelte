<script lang="ts">
  import { m } from '$lib/i18n/messages';

  let { url }: { url: string } = $props();
</script>

<!-- @component Inset server artwork with rounded corners and its original aspect ratio. -->
<div class="w-full px-2 pt-2">
  <img
    src={url}
    alt={m('media.server_banner_alt')}
    class="block h-auto w-full rounded-lg"
    onload={(event) => ((event.currentTarget as HTMLImageElement).style.display = '')}
    onerror={(event) => ((event.currentTarget as HTMLImageElement).style.display = 'none')}
  />
</div>
