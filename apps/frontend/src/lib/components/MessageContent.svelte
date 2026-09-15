<script lang="ts" module>
  // Re-export for tests
  export { renderMarkdown } from '$lib/markdown';
</script>

<script lang="ts">
  import { goto } from '$app/navigation';
  import { renderMarkdown as renderMd } from '$lib/markdown';
  import MarkdownHtml from '$lib/ui/MarkdownHtml.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { classifyMessageBodyChatLink } from '$lib/messageLinks';
  import { wrapValidMentions, type RoomMember } from '$lib/mentions';
  import { formatRelativeMessageTimestamp, wrapMessageTimestamps } from '$lib/messageTimestamps';
  import { parseTrustedMarkdownHtml } from '$lib/security/trustedHtml';
  import { getLocale } from '$lib/i18n/runtime';
  import { m } from '$lib/i18n/messages';
  import { formatDateTime, type TimeFormatSettings } from '$lib/utils/formatTime';
  import { SvelteDate } from 'svelte/reactivity';
  import Interval from '$lib/lifecycle/Interval.svelte';

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
    echoedToChannel = false,
    viewerLogin,
    timestampSettings = fallbackTimestampSettings,
    timestampLocale,
    onMentionClick
  }: {
    body: string;
    members?: RoomMember[];
    roleHandles?: string[];
    edited?: boolean;
    echoedToChannel?: boolean;
    viewerLogin?: string;
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

  function injectMessageStateMarkers(
    html: string,
    { edited, echoedToChannel }: { edited: boolean; echoedToChannel: boolean }
  ): string {
    const doc = parseTrustedMarkdownHtml(`<div>${html}</div>`);
    const root = doc.body.firstElementChild;
    if (!root) return html;
    const markers: HTMLElement[] = [];
    if (edited) {
      const badge = doc.createElement('span');
      badge.className =
        'edited-marker inline-block align-[-0.09em] leading-none whitespace-nowrap text-muted/70';
      badge.setAttribute('role', 'img');
      badge.setAttribute('aria-label', m('room.message.meta.edited'));
      badge.setAttribute('title', m('room.message.meta.edited'));
      const icon = doc.createElement('span');
      icon.className = 'iconify icon-[uil--pen] text-[0.875em]';
      icon.setAttribute('aria-hidden', 'true');
      badge.appendChild(icon);
      markers.push(badge);
    }
    if (echoedToChannel) {
      const badge = doc.createElement('span');
      badge.className =
        'echoed-to-channel-marker inline-block align-[-0.09em] leading-none whitespace-nowrap text-muted/70';
      badge.setAttribute('role', 'img');
      badge.setAttribute('aria-label', m('room.message.meta.echoed_to_channel'));
      badge.setAttribute('title', m('room.message.meta.echoed_to_channel'));
      const icon = doc.createElement('span');
      icon.className = 'iconify icon-[uil--megaphone] text-[0.875em]';
      icon.setAttribute('aria-hidden', 'true');
      badge.appendChild(icon);
      markers.push(badge);
    }
    if (markers.length === 0) return html;

    // Only inline the marker into a trailing <p> so it flows with the last word.
    // For block-level last children (<pre>, <ul>, <blockquote>) fall back to a
    // separate trailing line so the marker doesn't get clipped or look misplaced.
    const last = root.lastElementChild;
    if (last && last.tagName === 'P') {
      for (const marker of markers) {
        last.appendChild(doc.createTextNode(' '));
        last.appendChild(marker);
      }
    } else {
      const trailer = doc.createElement('p');
      for (const [index, marker] of markers.entries()) {
        if (index > 0) trailer.appendChild(doc.createTextNode(' '));
        trailer.appendChild(marker);
      }
      root.appendChild(trailer);
    }
    return root.innerHTML;
  }

  // Concealed spoilers are focusable reveal controls: Enter and Space activate
  // them like buttons (Space must not scroll the timeline).
  function handleContentKeydown(event: KeyboardEvent) {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    const concealedSpoiler = (event.target as HTMLElement).closest(
      'span.spoiler[data-spoiler]:not(.spoiler-revealed)'
    );
    if (!concealedSpoiler) return;
    event.preventDefault();
    event.stopPropagation();
    revealSpoiler(concealedSpoiler);
  }

  function enhanceSpoilerRegions(html: string): string {
    if (!html.includes('data-spoiler')) return html;

    const doc = parseTrustedMarkdownHtml(html);
    const spoilers = doc.body.querySelectorAll('span.spoiler[data-spoiler]');
    for (const spoiler of spoilers) {
      // Concealed region exposes itself as an explicit reveal control. The
      // wrapped body is excluded from selection and assistive tech until
      // reveal; see the .spoiler rules in prose.css.
      spoiler.setAttribute('role', 'button');
      spoiler.setAttribute('tabindex', '0');
      spoiler.setAttribute('aria-label', m('room.message.spoiler.reveal'));

      const body = doc.createElement('span');
      body.className = 'spoiler-body';
      body.setAttribute('aria-hidden', 'true');
      while (spoiler.firstChild) body.appendChild(spoiler.firstChild);
      spoiler.appendChild(body);

      const hint = doc.createElement('span');
      hint.className = 'spoiler-hint iconify icon-[mdi--eye-outline]';
      hint.setAttribute('aria-hidden', 'true');
      spoiler.appendChild(hint);
    }
    return doc.body.innerHTML;
  }

  function revealSpoiler(spoiler: Element | null | undefined): void {
    if (!spoiler || spoiler.classList.contains('spoiler-revealed')) return;
    spoiler.classList.add('spoiler-revealed');
    spoiler.removeAttribute('role');
    spoiler.removeAttribute('tabindex');
    spoiler.removeAttribute('aria-label');
    spoiler.querySelector('.spoiler-body')?.removeAttribute('aria-hidden');
  }

  // Render markdown then wrap valid mentions
  async function render(
    body: string,
    members: RoomMember[],
    roleHandles: string[],
    edited: boolean,
    echoedToChannel: boolean,
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
    const withSpoilers = enhanceSpoilerRegions(withTimestamps);
    return edited || echoedToChannel
      ? injectMessageStateMarkers(withSpoilers, { edited, echoedToChannel })
      : withSpoilers;
  }

  // Handle clicks on links (open in system browser), reveals of concealed
  // spoilers, and mentions (trigger callback).
  function handleContentClick(event: MouseEvent) {
    const target = event.target as HTMLElement;

    const concealedSpoiler = target.closest('span.spoiler[data-spoiler]:not(.spoiler-revealed)');
    if (concealedSpoiler) {
      event.preventDefault();
      revealSpoiler(concealedSpoiler);
      return;
    }

    const timestamp = target.closest('.message-timestamp') as HTMLButtonElement | null;
    if (timestamp) {
      const epochSeconds = Number(timestamp.dataset.timestampEpoch);
      if (!Number.isSafeInteger(epochSeconds)) return;
      event.preventDefault();
      const rect = timestamp.getBoundingClientRect();
      liveNow.setTime(Date.now());
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

<div class="prose max-w-none min-w-0" dir="auto" role="presentation" onclick={handleContentClick} onkeydown={handleContentKeydown}>
  {#await render(
    body,
    members,
    roleHandles,
    edited,
    echoedToChannel,
    viewerLogin,
    timestampSettings,
    timestampLocale
  )}
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
  <Interval milliseconds={1000} ontick={() => liveNow.setTime(Date.now())} />
  <ContextMenu
    anchor={activeTimestamp.anchor}
    role="dialog"
    ariaLabel={m('room.message.timestamp.details_title')}
    class="w-80"
    onclose={() => (activeTimestamp = null)}
  >
    <section class="menu-section px-3 py-2" data-testid="message-timestamp-details">
      <header class="mb-2 flex items-center gap-2 text-sm font-medium">
        <span class="iconify icon-[uil--clock] text-muted"></span>
        <span>{m('room.message.timestamp.details_title')}</span>
      </header>
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs">
        <dt class="text-muted">{m('room.message.timestamp.local_time')}</dt>
        <dd class="min-w-0 text-end break-words text-text">{activeTimestampLocalText}</dd>

        <dt class="text-muted">{m('room.message.timestamp.relative_time')}</dt>
        <dd class="min-w-0 text-end break-words text-text">{activeTimestampRelativeText}</dd>
      </dl>
    </section>
  </ContextMenu>
{/if}
