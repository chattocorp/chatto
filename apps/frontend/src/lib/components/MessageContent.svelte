<script lang="ts" module>
  // Re-export for tests
  export { rendererReady, renderMarkdown } from '$lib/markdown';
</script>

<script lang="ts">
  import { goto } from '$app/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { renderMarkdown as renderMd } from '$lib/markdown';
  import MarkdownHtml from '$lib/ui/MarkdownHtml.svelte';
  import { classifyMessageBodyChatLink } from '$lib/messageLinks';
  import { wrapValidMentions, type RoomMember } from '$lib/mentions';
  import { parseTrustedMarkdownHtml } from '$lib/security/trustedHtml';

  let {
    body,
    members = [],
    roleHandles = [],
    edited = false,
    onMentionClick
  }: {
    body: string;
    members?: RoomMember[];
    roleHandles?: string[];
    edited?: boolean;
    onMentionClick?: (userId: string, anchorRect: DOMRect) => void;
  } = $props();

  // The viewer's login on the active server, used by `wrapValidMentions` to
  // mark self-mentions. Same reactive registry-lookup pattern every other
  // chat-tree component uses — `tryGetStore` and the `?.` chain mean an
  // unregistered or pre-auth server leaves `viewerLogin` undefined, which
  // `wrapValidMentions` already treats as "no self-mention."
  const viewerLogin = $derived(
    serverRegistry.tryGetStore(getActiveServer())?.currentUser.user?.login
  );

  function injectEditedMarker(html: string): string {
    const doc = parseTrustedMarkdownHtml(`<div>${html}</div>`);
    const root = doc.body.firstElementChild;
    if (!root) return html;
    const badge = doc.createElement('span');
    badge.className = 'edited-marker text-xs whitespace-nowrap text-muted/70';
    badge.textContent = '(edited)';
    // Only inline the marker into a trailing <p> so it flows with the last word.
    // For block-level last children (<pre>, <ul>, <blockquote>) fall back to a
    // separate trailing line so the marker doesn't get clipped or look misplaced.
    const last = root.lastElementChild;
    if (last && last.tagName === 'P') {
      last.appendChild(doc.createTextNode(' '));
      last.appendChild(badge);
    } else {
      const trailer = doc.createElement('p');
      trailer.appendChild(badge);
      root.appendChild(trailer);
    }
    return root.innerHTML;
  }

  // Render markdown then wrap valid mentions
  async function render(
    body: string,
    members: RoomMember[],
    roleHandles: string[],
    edited: boolean,
    viewerLogin: string | undefined
  ): Promise<string> {
    const html = await renderMd(body);
    const wrapped = wrapValidMentions(html, members, viewerLogin, roleHandles);
    return edited ? injectEditedMarker(wrapped) : wrapped;
  }

  // Handle clicks on links (open in system browser) and mentions (trigger callback).
  function handleContentClick(event: MouseEvent) {
    const target = event.target as HTMLElement;

    // Check for mention clicks first
    const mention = target.closest('.mention') as HTMLElement | null;
    if (mention) {
      const userId = mention.dataset.userId;
      if (userId && onMentionClick) {
        event.preventDefault();
        onMentionClick(userId, mention.getBoundingClientRect());
      }
      return;
    }

    // Handle link clicks. Only allow-listed Chatto chat routes navigate in-app;
    // other same-origin URLs stay out-of-band to avoid message-link abuse.
    const anchor = target.closest('a');
    if (anchor?.href) {
      event.preventDefault();

      const chatLink = classifyMessageBodyChatLink(anchor.href);
      if (chatLink) {
        // eslint-disable-next-line svelte/no-navigation-without-resolve -- classifyMessageBodyChatLink returns an allow-listed resolved app path.
        goto(chatLink.path);
        return;
      }

      // External or non-allow-listed link → force opening in system browser.
      // target="_blank" alone is ignored by PWAs for same-origin URLs.
      // window.open() with features forces a new browser window.
      window.open(anchor.href, '_blank', 'noopener,noreferrer');
    }
  }
</script>

<div class="prose max-w-none min-w-0" role="presentation" onclick={handleContentClick}>
  {#await render(body, members, roleHandles, edited, viewerLogin)}
    {body}
  {:then html}
    <MarkdownHtml {html} />
  {:catch error}
    {body}
    {(() => {
      console.error('[MessageContent] Render failed:', error);
      return '';
    })()}
  {/await}
</div>
