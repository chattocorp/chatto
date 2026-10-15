<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import ComposerStatusRow from './ComposerStatusRow.svelte';
  import SlowModeBadge from './SlowModeBadge.svelte';
  import TypingIndicator from '../../../routes/chat/[serverId]/[roomId]/TypingIndicator.svelte';
  let {
    seconds = 30,
    remaining = 0,
    bypassed = false,
    typing = true,
    direction = 'ltr'
  }: {
    seconds?: number;
    remaining?: number;
    bypassed?: boolean;
    typing?: boolean;
    direction?: 'ltr' | 'rtl';
  } = $props();
</script>

<div class="w-60 max-w-full bg-background p-3 pt-8" dir={direction}>
  <div class="relative" data-testid="status-host">
    <ComposerStatusRow>
      <TypingIndicator
        typingUserIds={typing ? ['alice', 'bob', 'carol'] : []}
        members={[
          {
            id: 'alice',
            login: 'alice',
            displayName: 'A very long display name in a narrow pane',
            presenceStatus: PresenceStatus.ONLINE
          },
          { id: 'bob', login: 'bob', displayName: 'Bob', presenceStatus: PresenceStatus.ONLINE },
          {
            id: 'carol',
            login: 'carol',
            displayName: 'Carol',
            presenceStatus: PresenceStatus.ONLINE
          }
        ]}
      />
      <SlowModeBadge {seconds} {remaining} {bypassed} />
    </ComposerStatusRow>
    <div class="chat-input-surface px-3 py-4 text-muted">Type a message…</div>
  </div>
</div>
