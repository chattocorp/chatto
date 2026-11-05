import { fixturePng } from './imageFixture.ts';
/** Optional integration check against an explicitly selected disposable local server. */
import { fileURLToPath } from 'node:url';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import {
  check,
  click_test_id,
  find_test_id,
  focus,
  mount_headless,
  painted,
  screenshot,
  type,
  wait_for,
  finish,
  wheel,
  settle
} from 'gpuix-svelte/test';
import { dispatch } from 'gpuix-svelte';
import App from '../src/App.svelte';
import { createMessageAPI } from '@chatto/client/api/messages';
import { NativeSession } from '../src/session.svelte.ts';

const origin = process.env.CHATTO_GPUIX_TEST_ORIGIN;
if (!origin || !['127.0.0.1', 'localhost', '[::1]'].includes(new URL(origin).hostname))
  throw new Error('Set CHATTO_GPUIX_TEST_ORIGIN to a disposable loopback server');
const base = fileURLToPath(new URL('../../../.context/', import.meta.url));
mkdirSync(base, { recursive: true });
const directory = mkdtempSync(join(base, 'gpuix-live-'));
process.env.CHATTO_GPUIX_CONFIG_DIR = directory;
const session = new NativeSession({ password: async () => 'foobar123' });
session.origin = origin;
session.login = 'alice';
const mounted = mount_headless(App, { props: { session }, width: 1100, height: 500 });
try {
  await session.signIn();
  await wait_for(() => !!session.server?.roomList.rooms.length, {
    timeout: 20_000,
    label: 'SDK directory'
  });
  const server = session.server;
  if (!server) throw new Error('No SDK server');
  const room = server.roomList.rooms.find((room) => room.name === 'general');
  if (!room) throw new Error('The synthetic general room is missing');
  click_test_id(`room-${room.id}`);
  const timeline = server.rooms.messages(room.id);
  await wait_for(() => timeline.isCurrent && timeline.events.length > 0, {
    timeout: 15_000,
    label: 'SDK timeline'
  });
  check(
    'native sidebar uses the authenticated SDK directory',
    server.connection.status,
    'connected'
  );
  const input = find_test_id('live-draft');
  if (!input) throw new Error('The connected composer is missing');
  focus(input);
  type('g p u i x space s d k space s m o k e');
  click_test_id('live-send');
  await wait_for(() => painted().includes('gpuix sdk smoke'), {
    timeout: 15_000,
    label: 'live posted message'
  });
  check(
    'SDK message post and realtime delivery reach native Markdown',
    painted().includes('gpuix sdk smoke')
  );
  if (process.env.CHATTO_GPUIX_TEST_LONG_HISTORY === '1') {
    // Populate only the explicitly selected disposable server. Start paging
    // above the former 500-event UI limit to reproduce the reported cutoff.
    for (let index = 0; index < 550; index++) {
      await server.createMessage({ roomId: room.id, threadRootId: '' }, `History fixture ${index}`);
    }
    await wait_for(() => !timeline.isLoadingMore, { timeout: 15_000 });
    while (timeline.events.length < 500 && !timeline.hasReachedStart) {
      const before = timeline.events.length;
      await timeline.loadMore();
      if (timeline.events.length === before)
        throw new Error('Long-history fixture did not advance');
    }
    check('history regression starts with at least 500 events', timeline.events.length >= 500);
  }
  if (process.env.CHATTO_GPUIX_TEST_MEDIA === '1') {
    const png = fixturePng;
    await createMessageAPI(server.connection.apiConfig).createMessage({
      roomId: room.id,
      body: 'Native attachment smoke test',
      attachments: [new File([png], 'gpuix-fixture.png', { type: 'image/png' })]
    });
    await wait_for(() => !!find_test_id('media-image'), {
      timeout: 15_000,
      label: 'native attachment image'
    });

    check('realtime attachments reach the native image renderer', !!find_test_id('media-image'));
    const imageId = find_test_id('media-image')?.id;
    const list = find_test_id('live-timeline');
    if (!list) throw new Error('Missing live timeline');
    let stable = true;
    // A delayed paint report must not override the current sampled viewport.
    // Sample and report updates used to alternate image activation every 200ms.
    for (let sample = 0; sample < 3; sample++) {
      dispatch({ elementId: list.id, eventType: 'visibleRange', startIndex: 3, endIndex: 4 });
      settle();
      stable &&= find_test_id('media-image')?.id === imageId;
      await new Promise<void>((resolve) => setTimeout(resolve, 250));
      settle();
      stable &&= find_test_id('media-image')?.id === imageId;
    }
    check('stale paint ranges cannot evict a displayed image', stable);
    await timeline.refreshCurrentWindow();
    settle();
    check(
      'SDK metadata refresh retains the displayed image',
      find_test_id('media-image')?.id,
      imageId
    );
    if (process.env.CHATTO_GPUIX_SCREENSHOT) screenshot(process.env.CHATTO_GPUIX_SCREENSHOT);
  }
  const previous = timeline.events.length;
  const node = find_test_id('live-timeline');
  let anchor = '';
  if (node) {
    mounted.native.scrollToItem(node.id, 0, 0);
    settle();
    anchor = timeline.events.filter((event) => event.event.kind === 'messagePosted')[0]?.id ?? '';
    wheel({ x: 800, y: 250 }, 0, 500);
  }
  await wait_for(() => timeline.events.length > previous || timeline.hasReachedStart, {
    timeout: 15_000,
    label: 'older SDK page'
  });
  check(
    'top-of-list navigation pages through the SDK',
    timeline.events.length > previous || timeline.hasReachedStart
  );
  if (node && anchor) {
    await wait_for(
      () => {
        const offset = mounted.native.getListScrollTop(node.id);
        return (
          !!offset &&
          timeline.events.filter((event) => event.event.kind === 'messagePosted')[offset[0] ?? 0]
            ?.id === anchor
        );
      },
      { timeout: 5000, label: 'history anchor' }
    );
  }
  check('prepending history preserves the visible message', !!anchor);
  if (process.env.CHATTO_GPUIX_SCREENSHOT && process.env.CHATTO_GPUIX_TEST_MEDIA !== '1')
    screenshot(process.env.CHATTO_GPUIX_SCREENSHOT);
  await session.signOut();
  check('sign-out removes the SDK server from the UI', session.server, null);
} finally {
  mounted.unmount();
  session.close();
  rmSync(directory, { recursive: true, force: true });
}
finish(
  'Chatto GPUix live',
  5 +
    Number(process.env.CHATTO_GPUIX_TEST_LONG_HISTORY === '1') +
    3 * Number(process.env.CHATTO_GPUIX_TEST_MEDIA === '1')
);
