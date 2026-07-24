<script lang="ts" module>
  // Re-export for tests
  export { renderMarkdown } from '$lib/markdown';
</script>

<script lang="ts">
  import { goto } from '$app/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { renderMarkdown as renderMd } from '$lib/markdown';
  import MarkdownHtml from '$lib/ui/MarkdownHtml.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { classifyMessageBodyChatLink } from '$lib/messageLinks';
  import { wrapValidMentions, type RoomMember } from '$lib/mentions';
  import { formatRelativeMessageTimestamp, wrapMessageTimestamps } from '$lib/messageTimestamps';
  import { parseTrustedMarkdownHtml } from '$lib/security/trustedHtml';
  import { getLocale } from '$lib/i18n/runtime';
  import * as m from '$lib/i18n/messages';
  import { formatDateTime, type TimeFormatSettings } from '$lib/utils/formatTime';
  import { SvelteDate } from 'svelte/reactivity';

  const fallbackTimestampSettings: TimeFormatSettings = {
    get effectiveTimezone() {
      return undefined;
    },
    get effectiveHour12() {
      return undefined;
    }
  };
  type ActiveTimestamp = {
    epochSeconds: number;
    date: Date;
    anchor: { top: number; bottom: number; left: number };
  };

  let {
    body,
    members = [],
    roleHandles = [],
    edited = false,
    timestampSettings = fallbackTimestampSettings,
    timestampLocale,
    onMentionClick
  }: {
    body: string;
    members?: RoomMember[];
    roleHandles?: string[];
    edited?: boolean;
    timestampSettings?: TimeFormatSettings;
    timestampLocale?: string;
    onMentionClick?: (userId: string, anchorRect: DOMRect) => void;
  } = $props();
  let activeTimestamp = $state<ActiveTimestamp | null>(null);
  const liveNow = new SvelteDate();
  const activeTimestampLocale = $derived(timestampLocale ?? getLocale());
  const activeTimestampLocalText = $derived(
    activeTimestamp
      ? formatDateTime(activeTimestamp.date, timestampSettings, activeTimestampLocale)
      : ''
  );
  const activeTimestampRelativeText = $derived(
    activeTimestamp
      ? formatRelativeMessageTimestamp(activeTimestamp.date, activeTimestampLocale, liveNow)
      : ''
  );

  $effect(() => {
    if (!activeTimestamp) return;

    liveNow.setTime(Date.now());
    const interval = window.setInterval(() => {
      liveNow.setTime(Date.now());
    }, 1000);

    return () => {
      window.clearInterval(interval);
    };
  });

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
    viewerLogin: string | undefined,
    timestampSettings: TimeFormatSettings,
    timestampLocale: string | undefined
  ): Promise<string> {
    const html = await renderMd(body);
    const wrapped = wrapValidMentions(html, members, viewerLogin, roleHandles);
    const withTimestamps = wrapMessageTimestamps(
      wrapped,
      timestampSettings,
      timestampLocale ?? getLocale()
    );
    return edited ? injectEditedMarker(withTimestamps) : withTimestamps;
  }

  // Handle clicks on links (open in system browser) and mentions (trigger callback).
  function handleContentClick(event: MouseEvent) {
    const target = event.target as HTMLElement;

    const timestamp = target.closest('.message-timestamp') as HTMLButtonElement | null;
    if (timestamp) {
      const epochSeconds = Number(timestamp.dataset.timestampEpoch);
      if (!Number.isSafeInteger(epochSeconds)) return;
      event.preventDefault();
      const rect = timestamp.getBoundingClientRect();
      activeTimestamp = {
        epochSeconds,
        date: new Date(epochSeconds * 1000),
        anchor: { top: rect.top, bottom: rect.bottom, left: rect.left }
      };
      return;
    }

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
  {#await render(body, members, roleHandles, edited, viewerLogin, timestampSettings, timestampLocale)}
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

{#if activeTimestamp}
  <ContextMenu
    anchor={activeTimestamp.anchor}
    role="dialog"
    ariaLabel={m['room.message.timestamp.details_title']()}
    class="w-80"
    onclose={() => (activeTimestamp = null)}
  >
    <section class="menu-section px-3 py-2" data-testid="message-timestamp-details">
      <header class="mb-2 flex items-center gap-2 text-sm font-medium">
        <span class="iconify uil--clock text-muted"></span>
        <span>{m['room.message.timestamp.details_title']()}</span>
      </header>
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs">
        <dt class="text-muted">{m['room.message.timestamp.local_time']()}</dt>
        <dd class="min-w-0 text-right break-words text-text">{activeTimestampLocalText}</dd>

        <dt class="text-muted">{m['room.message.timestamp.relative_time']()}</dt>
        <dd class="min-w-0 text-right break-words text-text">{activeTimestampRelativeText}</dd>
      </dl>
    </section>
  </ContextMenu>
{/if}
