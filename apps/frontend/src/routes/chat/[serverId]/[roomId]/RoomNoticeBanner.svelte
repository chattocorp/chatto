<!-- @component A member-only notice banner with device-local, revision-scoped dismissal. -->
<script lang="ts">
  import { scale } from 'svelte/transition';
  import type { MarkdownMentionOptions } from '$lib/components/composer/mentionOptions';
  import type { RoomNoticeStore } from '@chatto/client/room/notice';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import RoomNoticePanel from './RoomNoticePanel.svelte';
  import { Panel, ScrollFader } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { COMPACT_MOTION_DURATION_MS, expoOutTransition } from '$lib/ui/motion';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import {
    formatDateTime,
    timeFormatSettingsFor,
    type TimeFormatSettings
  } from '$lib/utils/formatTime';
  import {
    readNoticeDismissal,
    roomNoticeDismissalKey,
    writeNoticeDismissal
  } from '$lib/storage/roomNoticeDismissal';

  let {
    store,
    serverId,
    viewerId,
    canManage = false,
    mentions,
    timeSettings = timeFormatSettingsFor(undefined)
  }: {
    store: RoomNoticeStore;
    serverId: string;
    viewerId: string;
    canManage?: boolean;
    mentions?: MarkdownMentionOptions;
    timeSettings?: TimeFormatSettings;
  } = $props();
  const key = $derived(roomNoticeDismissalKey(serverId, viewerId, store.roomId));
  let dismissed = $derived(readNoticeDismissal(key));
  const notice = $derived(store.notice);
  const publishedAt = $derived(notice?.publishedAt?.toDate());
  const publicationLabel = $derived(
    publishedAt ? formatDateTime(publishedAt, timeSettings, getReactiveLocale()) : ''
  );

  function dismiss() {
    dismissed = notice?.publicationId ?? '';
    writeNoticeDismissal(key, dismissed);
  }

  /** Freeze the outgoing surface so the conversation can reclaim its space immediately. */
  function dismissTransition(node: HTMLElement) {
    const { offsetWidth, offsetHeight, offsetLeft, offsetTop } = node;
    Object.assign(node.style, {
      width: `${offsetWidth}px`,
      height: `${offsetHeight}px`,
      left: `${offsetLeft}px`,
      top: `${offsetTop}px`,
      position: 'absolute',
      pointerEvents: 'none',
      zIndex: '1'
    });
    return scale(node, {
      ...expoOutTransition(COMPACT_MOTION_DURATION_MS),
      start: 0.96,
      opacity: 0
    });
  }
</script>

{#snippet publicationTime()}
  {#if publishedAt}
    <time class="block truncate" datetime={publishedAt.toISOString()} title={publicationLabel}>
      {publicationLabel}
    </time>
  {/if}
{/snippet}

{#snippet banner(open?: () => void)}
  {#if notice?.body}
    <section
      class="relative flex min-h-0 flex-col md:max-h-[min(40dvh,24rem)]"
      aria-label={m('room.notice.title')}
    >
      {#if dismissed !== notice.publicationId}
        <div
          class="flex min-h-0 flex-1 origin-top flex-col px-3 pt-2"
          in:scale|global={{
            ...expoOutTransition(COMPACT_MOTION_DURATION_MS),
            start: 0.96,
            opacity: 0
          }}
          out:dismissTransition|global
        >
          <Panel
            title={m('room.notice.title')}
            subtitle={publishedAt ? publicationTime : undefined}
            fillHeight
          >
            {#snippet actions()}
              {#if open}
                <Button variant="secondary" size="compact" onclick={open}>{m('common.edit')}</Button
                >
              {/if}
              <Button variant="action" size="compact" onclick={dismiss}
                >{m('room.notice.dismiss')}</Button
              >
            {/snippet}
            <ScrollFader
              top
              bottom
              class="min-h-0 min-w-0 flex-1 text-text"
              aria-label={m('room.notice.title')}
              dir="auto"
            >
              <MessageContent
                body={notice.body}
                members={mentions?.members}
                roleHandles={mentions?.roles?.map((role) => role.name)}
                viewerLogin={mentions?.viewerLogin}
                onMentionClick={mentions?.onMentionClick}
              />
            </ScrollFader>
          </Panel>
        </div>
      {/if}
    </section>
  {/if}
{/snippet}

<!-- Keep one render path across permission changes so Shield Mode does not
     remount the banner. The editor also outlives remote notice clears. -->
<RoomNoticePanel {store} {serverId} {canManage} {mentions}>
  {#snippet trigger(open)}{@render banner(open)}{/snippet}
</RoomNoticePanel>
