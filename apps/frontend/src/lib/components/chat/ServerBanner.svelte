<script lang="ts">
  import { m } from '$lib/i18n/messages';

  let { url }: { url: string } = $props();
</script>

<!-- @component Framed server artwork in the sidebar, preserving its original aspect ratio. -->
<div class="w-full px-2 pt-2">
  <div class="inset-frame">
    <div class="overflow-hidden inset-content">
      <img
        src={url}
        alt={m('media.server_banner_alt')}
        class="block h-auto w-full"
        onload={(event) => ((event.currentTarget as HTMLImageElement).style.display = '')}
        onerror={(event) => ((event.currentTarget as HTMLImageElement).style.display = 'none')}
      />
    </div>
  </div>
</div>
