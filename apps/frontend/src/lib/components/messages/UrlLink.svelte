<!-- @component An automatic web link with stable URL text. Preview metadata never changes its layout. -->
<script lang="ts">
  import { parseWebLink } from '$lib/navigation/webLink';
  import { handleRenderedLinkClick, renderedLinkTarget } from '$lib/navigation/renderedLink';

  let { href }: { href: string } = $props();
  const url = $derived(parseWebLink(href));
  const destination = $derived(renderedLinkTarget(href));
</script>

{#if url}
  <a
    {...destination}
    class="url-link"
    data-content-href={href}
    title={href}
    onclick={(event) => handleRenderedLinkClick(event, href)}><bdi dir="ltr">{href}</bdi></a
  >
{/if}
