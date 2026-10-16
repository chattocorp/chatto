<!-- @component A short link to the server's compatibility report. Details belong on that page. -->
<script lang="ts">
  import type { ServerFeature } from '@chatto/client/server/compatibility';
  import { resolve } from '$app/paths';
  import { m } from '$lib/i18n/messages';
  let { features, serverSegment }: { features: readonly ServerFeature[]; serverSegment: string } =
    $props();
</script>

{#if features.length > 0}
  <div class="flex items-start gap-2 text-warning" data-testid="feature-compatibility-notice">
    <span class="iconify mt-0.5 icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"></span>
    <p>
      {m('chat.compatibility.limited')}
      <a
        class="link"
        href={resolve(
          `/chat/${serverSegment}/info#${features.length === 1 ? features[0] : 'compatibility'}`
        )}
      >
        {m('chat.compatibility.view')}
      </a>
    </p>
  </div>
{/if}
