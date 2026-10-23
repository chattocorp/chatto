<!--
@component

Room metadata, description, and pins in independently collapsible sidebar sections.
The sections share one scroll viewport and retain their disclosure preferences.
-->
<script lang="ts">
  import { serverStorageKey } from '@chatto/client/storage/serverStorage';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import RoomName from '$lib/components/chat/RoomName.svelte';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { formatRelativeMessageTimestamp } from '$lib/messageTimestamps';
  import type { RoomPinsStore } from '$lib/state/room';
  import { ScrollFader } from '$lib/ui';
  import RoomPinsPanel from './RoomPinsPanel.svelte';

  let {
    serverId,
    roomName,
    roomDescription,
    roomCreatedAt = null,
    memberCount = null,
    isUniversal = false,
    pinsStore,
    onOpenPin
  }: {
    serverId: string;
    roomName: string;
    roomDescription: string;
    roomCreatedAt?: string | null;
    /** Null means that the first member page has not loaded yet. */
    memberCount?: number | null;
    isUniversal?: boolean;
    pinsStore?: RoomPinsStore;
    onOpenPin?: (messageEventId: string, threadRootEventId: string | null) => void;
  } = $props();
  let now = $state(new Date());
  const createdAt = $derived.by(() => {
    if (!roomCreatedAt) return null;
    const date = new Date(roomCreatedAt);
    return Number.isFinite(date.getTime()) ? date : null;
  });
</script>

<Interval milliseconds={60_000} ontick={() => (now = new Date())} />
<ScrollFader top bottom keyboardFocusable={false} class="min-h-0 flex-1">
  <RoomGroupSection
    label={m('room.information.basic')}
    items={[]}
    persistKey={serverStorageKey(serverId, 'collapsible:room-information:basic')}
  >
    {#snippet content()}
      <div class="space-y-2 px-2 py-3">
        <p class="font-semibold"><RoomName name={roomName} {isUniversal} /></p>
        <dl class="grid grid-cols-[auto_minmax(0,1fr)] items-baseline gap-x-4 gap-y-2 pt-3 text-sm">
          <dt class="text-muted">{m('room.information.created')}</dt>
          <dd class="min-w-0 text-end">
            {#if createdAt}
              <time
                datetime={createdAt.toISOString()}
                title={createdAt.toLocaleString(getReactiveLocale())}
              >
                {formatRelativeMessageTimestamp(createdAt, getReactiveLocale(), now)}
              </time>
            {:else}
              {m('common.unknown')}
            {/if}
          </dd>
          <dt class="text-muted">{m('room.sidebar.members')}</dt>
          <dd class="min-w-0 text-end tabular-nums">
            {memberCount === null
              ? m('common.unknown')
              : memberCount.toLocaleString(getReactiveLocale())}
          </dd>
        </dl>
      </div>
    {/snippet}
  </RoomGroupSection>
  {#if roomDescription.trim()}
    <RoomGroupSection
      label={m('room.information.description')}
      items={[]}
      persistKey={serverStorageKey(serverId, 'collapsible:room-information:description')}
      separated
    >
      {#snippet content()}
        <div class="min-w-0 px-2 py-3 text-muted">
          <MessageContent body={roomDescription} />
        </div>
      {/snippet}
    </RoomGroupSection>
  {/if}
  <RoomGroupSection
    label={m('room.information.pinned_messages')}
    items={[]}
    persistKey={serverStorageKey(serverId, 'collapsible:room-information:pins')}
    separated
  >
    {#snippet content()}
      {#if pinsStore}
        <RoomPinsPanel store={pinsStore} {onOpenPin} />
      {/if}
    {/snippet}
  </RoomGroupSection>
</ScrollFader>
