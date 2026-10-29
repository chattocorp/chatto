<!-- @component A compact automatic web link. Its full URL remains the destination and accessible label. -->
<script lang="ts">
  import { parseWebLink } from '$lib/navigation/webLink';
  import { handleRenderedLinkClick, renderedLinkTarget } from '$lib/navigation/renderedLink';

  let { href }: { href: string } = $props();
  const url = $derived(parseWebLink(href));
  const destination = $derived(renderedLinkTarget(href));
  const suffix = $derived(
    url ? `${url.pathname === '/' ? '' : url.pathname}${url.search}${url.hash}` : ''
  );
</script>

{#if url}
  <a
    {...destination}
    class="url-link"
    data-content-href={href}
    title={href}
    aria-label={href}
    onclick={(event) => handleRenderedLinkClick(event, href)}
    onauxclick={(event) => {
      if (event.button === 1) handleRenderedLinkClick(event, href);
    }}
  >
    <span class="iconify icon-[uil--external-link-alt] shrink-0 self-center" aria-hidden="true"
    ></span>
    <bdi class="min-w-0 truncate" dir="ltr">{url.host}<span class="text-muted">{suffix}</span></bdi>
  </a>
{/if}
