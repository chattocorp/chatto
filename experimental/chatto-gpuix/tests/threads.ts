/** Native thread controls enforce authority and deadlines before submitting a reply. */
import {
  all_text,
  check,
  click_test_id,
  find_test_id,
  finish,
  focus,
  mount_headless,
  painted,
  type,
  wait_for
} from 'gpuix-svelte/test';
import type { Server } from '@chatto/client';
import { TimelineEventKind, type TimelineEventView } from '@chatto/client/timeline/timelineEvents';
import ThreadPanel from '../src/ThreadPanel.svelte';
import ThemeHarness from './ThemeHarness.svelte';
let requests = 0;
let closes = 0;
let failed = false;
const destinations: { roomId: string; threadRootId: string }[] = [];
function fixture(canReply: boolean, deadline: string | null = null) {
  const root: TimelineEventView = {
    id: 'root',
    createdAt: new Date().toISOString(),
    actor: null,
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room',
      body: 'Fixture root',
      attachments: [],
      reactions: [],
      replyCount: 0,
      threadParticipants: [],
      canReplyInThread: canReply
    }
  };
  const timeline = {
    threadEvents: [root],
    isCurrent: true,
    isInitialLoading: false,
    isLoadingMore: false,
    hasReachedStart: true,
    threadSlowModeNextPostAt: deadline
  };
  const server = {
    serverUrl: 'https://fixture.invalid',
    projection: { rooms: new Map() },
    roomList: { rooms: [{ id: 'room', name: 'Fixture' }] },
    rooms: { thread: () => timeline },
    async createMessage(target: { roomId: string; threadRootId: string }, _body: string) {
      requests++;
      destinations.push(target);
      if (failed) throw new Error('Synthetic send failure');
      return { id: 'reply' };
    }
  } as unknown as Server;
  return mount_headless(ThemeHarness, {
    props: {
      component: ThreadPanel,
      componentProps: {
        server,
        roomId: 'room',
        rootId: 'root',
        compact: false,
        onClose: () => closes++
      }
    },
    width: 900,
    height: 500
  });
}
let mounted = fixture(false);
try {
  check(
    'read-only thread explains unavailable replies',
    all_text().includes('Replies are not available in this thread.')
  );
  focus(find_test_id('thread-draft')!);
  type('f i x t u r e');
  click_test_id('thread-send');
  check('read-only thread sends no request', requests, 0);
} finally {
  await mounted.unmount();
}
mounted = fixture(true, new Date(Date.now() + 60_000).toISOString());
try {
  check(
    'slow mode shows the current wait',
    all_text().some((text) => text.includes('before replying.'))
  );
  focus(find_test_id('thread-draft')!);
  type('f i x t u r e');
  click_test_id('thread-send');
  check('slow mode prevents early submission', requests, 0);
} finally {
  await mounted.unmount();
}
failed = true;
mounted = fixture(true);
try {
  focus(find_test_id('thread-draft')!);
  type('f i x t u r e');
  click_test_id('thread-send');
  await wait_for(() =>
    all_text().some((text) => text.includes('The reply could not be confirmed.'))
  );
  check('failed sends expose a retry-safe error', requests, 1);
  check('failed sends retain the draft', painted().includes('fixture'));
  check(
    'reply is scoped to its room and thread',
    destinations[0]?.roomId === 'room' && destinations[0]?.threadRootId === 'root'
  );
  click_test_id('thread-close');
  check('close action returns control to the parent', closes, 1);
} finally {
  await mounted.unmount();
}
finish('Chatto GPUix thread controls', 8);
