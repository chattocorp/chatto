<!--
@component
Shows member-authored web links. Inspect URLs locally so opening a profile
does not contact the linked sites.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';

  let { links = [], class: className = '' }: { links?: string[]; class?: string } = $props();

  function displayLink(value: string) {
    let url: URL;
    try {
      url = new URL(value);
    } catch {
      return [];
    }
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) return [];
    const profile =
      url.origin === 'https://bsky.app' && /^\/profile\/([^/]+)\/?$/.exec(url.pathname);
    return [{ href: url.href, label: profile ? `Bluesky · ${profile[1]}` : url.href }];
  }

  const visible = $derived(links.flatMap(displayLink));
</script>

{#if visible.length}
  <section aria-label={m('chat.profile.links')} class={className}>
    <h3 class="mb-1 text-xs font-semibold text-muted">{m('chat.profile.links')}</h3>
    <ul class="space-y-1 text-sm">
      {#each visible as link, i (`${i}:${link.href}`)}
        <li>
          <a
            href={link.href}
            target="_blank"
            rel="noopener noreferrer"
            class="break-all text-link underline">{link.label}</a
          >
        </li>
      {/each}
    </ul>
  </section>
{/if}
