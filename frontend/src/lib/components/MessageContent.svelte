<script lang="ts" module>
  // Re-export for tests
  export { rendererReady, renderMarkdown } from '$lib/markdown';
</script>

<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- goto target is built via buildMessageLinkPath which already calls resolve() */
  import { goto } from '$app/navigation';
  import { getCurrentUser, type CurrentUserState } from '$lib/auth/currentUser.svelte';
  import { renderMarkdown as renderMd } from '$lib/markdown';
  import { parseMessageLink, buildMessageLinkPath } from '$lib/messageLinks';
  import { wrapValidMentions, type RoomMember } from '$lib/mentions';

  let {
    body,
    members = [],
    onMentionClick
  }: {
    body: string;
    members?: RoomMember[];
    onMentionClick?: (userId: string, anchorRect: DOMRect) => void;
  } = $props();

  // getCurrentUser throws if context is not set (e.g., in tests), so handle gracefully
  let currentUser: CurrentUserState | undefined;
  try {
    currentUser = getCurrentUser();
  } catch {
    // Context not available - self-mention highlighting won't work
  }

  // Render markdown then wrap valid mentions
  async function render(body: string, members: RoomMember[]): Promise<string> {
    const html = await renderMd(body);
    return wrapValidMentions(html, members, currentUser?.user?.login);
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

    // Handle link clicks — Chatto message links navigate in-app,
    // all other links open in the system browser (PWA compatibility).
    const anchor = target.closest('a');
    if (anchor?.href) {
      event.preventDefault();

      // Internal message link → navigate in-app via SvelteKit
      const messageLink = parseMessageLink(anchor.href);
      if (messageLink?.instanceId) {
        goto(buildMessageLinkPath(messageLink.instanceId, messageLink.spaceId, messageLink.roomId, messageLink.messageId));
        return;
      }

      // External link → force opening in system browser.
      // target="_blank" alone is ignored by PWAs for same-origin URLs.
      // window.open() with features forces a new browser window.
      window.open(anchor.href, '_blank', 'noopener,noreferrer');
    }
  }
</script>

<div class="prose max-w-none min-w-0" role="presentation" onclick={handleContentClick}>
  {#await render(body, members)}
    <!-- Show escaped body while loading -->
    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
    {@html body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}
  {:then html}
    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
    {@html html}
  {:catch error}
    <!-- Render failed - show escaped body as fallback -->
    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
    {@html body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}
    {(() => {
      console.error('[MessageContent] Render failed:', error);
      return '';
    })()}
  {/await}
</div>
