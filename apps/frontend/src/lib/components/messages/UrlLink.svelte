<!-- @component A compact automatic web link. Its full URL remains the destination and accessible description. -->
<script lang="ts">
  import { parseWebLink } from '$lib/navigation/webLink';
  import { handleRenderedLinkClick, renderedLinkTarget } from '$lib/navigation/renderedLink';

  let { href }: { href: string } = $props();
  const descriptionId = $props.id();
  const url = $derived(parseWebLink(href));
  const destination = $derived(renderedLinkTarget(href));
  const path = $derived.by(() => {
    if (!url || url.pathname === '/') return '';
    const parts = url.pathname.split('/').filter(Boolean);
    const last = parts.at(-1) ?? '';
    // Hide opaque identifiers without making assumptions about the website.
    const opaque = /^[a-z\d]{14,}$/i.test(last) && /[a-z]/i.test(last) && /\d/.test(last);
    if (opaque) return parts.length > 1 ? `/${parts[0]}/…` : '/…';
    if (url.pathname.length > 24 && parts.length > 1) return `/…/${last}`;
    return url.pathname;
  });
</script>

{#if url}
  <a
    {...destination}
    class="url-link"
    data-content-href={href}
    title={href}
    aria-describedby={descriptionId}
    onclick={(event) => handleRenderedLinkClick(event, href)}
    onauxclick={(event) => {
      if (event.button === 1) handleRenderedLinkClick(event, href);
    }}
  >
    <bdi class="flex min-w-0" dir="ltr">
      <span class="min-w-0 flex-1 truncate">{url.host}</span>{#if path}<span
          class="max-w-[60%] shrink-0 truncate text-muted">{path}</span
        >{/if}
    </bdi>
  </a>
  <span id={descriptionId} hidden>{href}</span>
{/if}
