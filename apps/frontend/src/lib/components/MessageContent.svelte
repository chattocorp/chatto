<script lang="ts" module>
  // Re-export for tests
  export { renderMarkdown } from '$lib/markdown';
</script>

<script lang="ts">
  import { renderMarkdown as renderMd } from '$lib/markdown';
  import { MarkdownHtml, ContextMenu } from '$lib/ui';
  import { extractMentions, resolveRenderedMentions, type RoomMember } from '$lib/mentions';
  import type { RoomMembersStore } from '@chatto/client/room/members';
  import type { Attachment } from 'svelte/attachments';
  import { untrack } from 'svelte';
  import { formatRelativeMessageTimestamp, wrapMessageTimestamps } from '$lib/messageTimestamps';
  import { parseTrustedMarkdownHtml } from '$lib/security/trustedHtml';
  import { getLocale } from '$lib/i18n/runtime';
  import { m } from '$lib/i18n/messages';
  import { formatDateTime, type TimeFormatSettings } from '$lib/utils/formatTime';
  import { SvelteDate } from 'svelte/reactivity';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { getAllContexts } from 'svelte';
  import { enhanceEntityLinks } from './messages/enhanceEntityLinks';

  const enhanceLink = enhanceEntityLinks(getAllContexts());

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
    mentionStore,
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
    /** Room lookup owner; retain handles only while this content is mounted. */
    mentionStore?: RoomMembersStore;
    roleHandles?: string[];
    edited?: boolean;
    echoedToChannel?: boolean;
    viewerLogin?: string;
    timestampSettings?: TimeFormatSettings;
    timestampLocale?: string;
    onMentionClick?: (userId: string, anchorRect: DOMRect) => void;
  } = $props();
  const resolvedMembers = $derived(mentionStore?.mentionMembers ?? members);
  const mentionHandles = $derived(
    extractMentions(body).filter(
      (handle) =>
        !['all', 'here', ...roleHandles.map((role) => role.toLowerCase())].includes(
          handle.toLowerCase()
        )
    )
  );
  const retainMentions: Attachment = () => {
    const store = mentionStore;
    const handles = mentionHandles;
    // This subscription belongs to the rendered content. Its internal cache
    // reads must not cause the attachment to release and retain it again.
    return untrack(() => store?.retainMentions(handles));
  };
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

  // Render markdown, then resolve the mention candidates that it marked
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
    const html = await renderMd(body, { mentions: true });
    const wrapped = resolveRenderedMentions(html, members, viewerLogin, roleHandles);
    const withTimestamps = wrapMessageTimestamps(
      wrapped,
      timestampSettings,
      timestampLocale ?? getLocale()
    );
    return edited || echoedToChannel
      ? injectMessageStateMarkers(withTimestamps, { edited, echoedToChannel })
      : withTimestamps;
  }

  // Message-specific actions; MarkdownHtml owns link navigation and code copy.
  function handleContentClick(event: MouseEvent) {
    const target = event.target as HTMLElement;

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
  }
</script>

<div
  {@attach retainMentions}
  class="prose max-w-none min-w-0"
  dir="auto"
  role="presentation"
  onclick={handleContentClick}
>
  {#await render(body, resolvedMembers, roleHandles, edited, echoedToChannel, viewerLogin, timestampSettings, timestampLocale)}
    {body}
  {:then html}
    <MarkdownHtml {html} {enhanceLink} />
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
      <header class="mb-2 flex items-center gap-2 font-medium">
        <span aria-hidden="true" class="iconify icon-[uil--clock] text-muted"></span>
        <span>{m('room.message.timestamp.details_title')}</span>
      </header>
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1">
        <dt class="text-muted">{m('room.message.timestamp.local_time')}</dt>
        <dd class="min-w-0 text-end break-words text-text">{activeTimestampLocalText}</dd>

        <dt class="text-muted">{m('room.message.timestamp.relative_time')}</dt>
        <dd class="min-w-0 text-end break-words text-text">{activeTimestampRelativeText}</dd>
      </dl>
    </section>
  </ContextMenu>
{/if}
