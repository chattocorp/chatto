<!-- @component A compact automatic web link. Its full URL remains the destination and accessible description. -->
<script lang="ts">
  import { parseWebLink } from '$lib/navigation/webLink';
  import { handleRenderedLinkClick, renderedLinkTarget } from '$lib/navigation/renderedLink';

  let { href, title }: { href: string; title?: string } = $props();
  const descriptionId = $props.id();
  const url = $derived(parseWebLink(href));
  const destination = $derived(renderedLinkTarget(href));
  const hasTitle = $derived(Boolean(title?.trim()));
  const host = $derived.by(() => {
    if (!url) return '';
    const port = url.port ? `:${url.port}` : '';
    const budget = 22 - port.length;
    // Keep the domain ending and port visible instead of clipping them off.
    if (url.hostname.length <= budget) return url.host;
    const leading = Math.floor((budget - 1) / 3);
    return `${url.hostname.slice(0, leading)}…${url.hostname.slice(-(budget - leading - 1))}${port}`;
  });
  const path = $derived.by(() => {
    if (!url || url.pathname === '/') return '';
    const parts = url.pathname.split('/').filter(Boolean);
    const last = parts.at(-1) ?? '';
    // Hide opaque identifiers without making assumptions about the website.
    const opaque = /^[a-z\d]{14,}$/i.test(last) && /[a-z]/i.test(last) && /\d/.test(last);
    // A hostname is more useful than two independently truncated URL fragments.
    // Keep only short, complete paths; the hover card shows the full destination.
    if (opaque || url.pathname.length > 24 || url.host.length + url.pathname.length > 32) return '';
    return url.pathname;
  });
</script>

{#if url}
  <a
    {...destination}
    class="url-link"
    class:url-link-with-title={hasTitle}
    data-content-href={href}
    title={href}
    aria-describedby={descriptionId}
    onclick={(event) => handleRenderedLinkClick(event, href)}
    onauxclick={(event) => {
      if (event.button === 1) handleRenderedLinkClick(event, href);
    }}
  >
    <span class="url-link-main">
      {#if destination.target === '_blank'}
        <span class="iconify icon-[uil--external-link-alt] shrink-0 self-center" aria-hidden="true"
        ></span>
      {/if}
      {#if hasTitle}
        <bdi class="min-w-0 truncate">{title}</bdi>
      {:else}
        <bdi class="flex min-w-0" dir="ltr">
          <span class="min-w-0 flex-1 truncate">{host}</span>{#if path}<span
              class="shrink-0 text-muted">{path}</span
            >{/if}
        </bdi>
      {/if}
    </span>
    {#if hasTitle}
      <span class="url-link-host"><bdi class="truncate" dir="ltr">{host}</bdi></span>
    {/if}
  </a>
  <span id={descriptionId} hidden>{href}</span>
{/if}
