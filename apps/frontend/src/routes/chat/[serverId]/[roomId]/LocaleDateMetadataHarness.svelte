<script lang="ts">
  import { getLocale } from '$lib/i18n/runtime';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import {
    TimelineEventKind,
    type TimelineEventView
  } from '$lib/render/timelineEvents';
  import type { UserSettingsState } from '$lib/state/userSettings.svelte';
  import { computeEventMetadata } from './messageGrouping';

  const settings = {
    get effectiveTimezone(): string | undefined {
      return 'UTC';
    },
    get effectiveHour12(): boolean | undefined {
      return undefined;
    }
  } as unknown as UserSettingsState;

  const events = [
    {
      id: 'evt-1',
      createdAt: '2025-11-20T10:00:00Z',
      actorId: 'u_alice',
      actor: {
        id: 'u_alice',
        login: 'alice',
        displayName: 'Alice',
        deleted: false,
        presenceStatus: PresenceStatus.ONLINE,
        avatarUrl: null
      },
      event: {
        kind: TimelineEventKind.MessagePosted,
        roomId: 'r_test',
        body: 'Hello',
        attachments: [],
        linkPreview: null,
        reactions: [],
        updatedAt: null,
        inReplyTo: null,
        threadRootEventId: null,
        replyCount: 0,
        lastReplyAt: null,
        threadParticipants: [],
        viewerIsFollowingThread: null
      }
    }
  ] as unknown as TimelineEventView[];

  const activeLocale = $derived(getLocale());
  const metadata = $derived(computeEventMetadata(events, settings, activeLocale));
</script>

<div data-testid="day-label">{metadata[0]?.dayLabel}</div>
