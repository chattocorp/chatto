/** Optional native thread integration against a disposable loopback server. */
import {
  bounds,
  check,
  click_test_id,
  find_test_id,
  finish,
  focus,
  mount_headless,
  painted,
  screenshot,
  settle,
  type,
  wait_for
} from 'gpuix-svelte/test';
import App from '../src/App.svelte';
import { NativeSession } from '../src/session.svelte.ts';
import { memoryState } from '../src/state.ts';
import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
const origin = process.env.CHATTO_GPUIX_TEST_ORIGIN;
if (!origin || !['127.0.0.1', 'localhost', '[::1]'].includes(new URL(origin).hostname))
  throw new Error('Select a disposable loopback server');
const originalError = console.error;
let unsafeMutation = false;
console.error = (...args: unknown[]) => {
  unsafeMutation ||= args.some((value) => String(value).includes('state_unsafe_mutation'));
  originalError(...args);
};
const session = new NativeSession({
  openState: async () => memoryState(),
  password: async () => 'foobar123'
});
session.origin = origin;
session.login = 'alice';
const mounted = mount_headless(App, { props: { session }, width: 1400, height: 538 });
try {
  await session.signIn();
  await wait_for(() => !!session.server?.roomList.rooms.length, { timeout: 20_000 });
  const server = session.server!;
  const room = server.roomList.rooms.find((item) => item.name === 'general')!;
  click_test_id(`room-${room.id}`);
  const roomStore = server.rooms.messages(room.id);
  await wait_for(() => roomStore.isCurrent, { timeout: 15_000 });
  focus(find_test_id('live-draft')!);
  type('c h a n n e l space d r a f t');
  const root = await server.createMessage(
    { roomId: room.id, threadRootId: '' },
    '**Native thread root**'
  );
  await wait_for(() => !!find_test_id(`thread-open-${root.id}`), { timeout: 15_000 });
  const roomList = find_test_id('live-timeline')!;
  mounted.native.scrollToItem(
    roomList.id,
    roomStore.rootEvents
      .filter((item) => isMessagePostedEvent(item.event))
      .findIndex((item) => item.id === root.id),
    0
  );
  settle();
  click_test_id(`thread-open-${root.id}`);
  const thread = server.rooms.thread(room.id, root.id);
  await wait_for(() => thread.isCurrent && !!find_test_id('thread-draft'), { timeout: 15_000 });
  check('message action opens the SDK thread timeline', !!find_test_id('thread-panel'));
  const headers = ['server-pane-header', 'channel-pane-header', 'thread-pane-header'].map((id) =>
    bounds(find_test_id(id)!)!
  );
  check(
    'all three pane headers share a baseline and height',
    headers.every(
      (header) =>
        Math.abs(header.y - headers[0]!.y) < 0.1 &&
        Math.abs(header.height - headers[0]!.height) < 0.1
    )
  );
  check(
    'thread includes the root message',
    thread.threadEvents.some((item) => item.id === root.id)
  );
  focus(find_test_id('thread-draft')!);
  type('n a t i v e space t h r e a d space r e p l y');
  click_test_id('thread-send');
  await wait_for(
    () =>
      thread.threadEvents.some(
        (item) => isMessagePostedEvent(item.event) && item.event.body === 'native thread reply'
      ),
    { timeout: 15_000, label: 'native reply send' }
  );
  check('native composer sends a reply to the selected thread', true);
  const external = await server.createMessage(
    { roomId: room.id, threadRootId: root.id },
    'Realtime thread reply'
  );
  await wait_for(
    () =>
      thread.threadEvents.some((item) => item.id === external.id) &&
      painted().includes('Realtime thread reply'),
    { timeout: 15_000 }
  );
  check('thread replies render from realtime delivery', true);
  await wait_for(
    () => {
      const row = roomStore.rootEvents.find((item) => item.id === root.id);
      return !!row && isMessagePostedEvent(row.event) && row.event.replyCount >= 2;
    },
    { timeout: 15_000 }
  );
  check('room root receives the realtime reply count', true);
  check(
    'thread replies stay out of the channel timeline',
    !roomStore.rootEvents.some((item) => item.id === external.id)
  );
  if (process.env.CHATTO_GPUIX_SCREENSHOT) screenshot(process.env.CHATTO_GPUIX_SCREENSHOT);
  focus(find_test_id('thread-draft')!);
  type('u n s e n t space r e p l y');
  session.setUiScale(1.5);
  settle();
  check(
    'thread survives scaling into the compact layout',
    !!find_test_id('thread-panel') && !find_test_id('live-timeline')
  );
  const scaledServerHeader = bounds(find_test_id('server-pane-header')!)!;
  const scaledThreadHeader = bounds(find_test_id('thread-pane-header')!)!;
  check(
    'compact thread and sidebar headers remain aligned',
    Math.abs(scaledServerHeader.height - scaledThreadHeader.height) < 0.1 &&
      Math.abs(scaledServerHeader.y - scaledThreadHeader.y) < 0.1
  );
  check('thread draft survives scaling', painted().includes('unsent reply'));
  click_test_id('thread-close');
  session.setUiScale(1);
  settle();
  check(
    'closing returns to the channel and its draft',
    !find_test_id('thread-panel') && painted().includes('channel draft')
  );
  mounted.native.scrollToItem(
    find_test_id('live-timeline')!.id,
    roomStore.rootEvents
      .filter((item) => isMessagePostedEvent(item.event))
      .findIndex((item) => item.id === root.id),
    0
  );
  settle();
  click_test_id(`thread-open-${root.id}`);
  const other = server.roomList.rooms.find((item) => item.id !== room.id)!;
  click_test_id(`room-${other.id}`);
  check(
    'switching rooms closes the thread and discards its draft',
    !find_test_id('thread-panel') && !painted().includes('unsent reply')
  );
  check('thread navigation does not mutate reactive state during rendering', !unsafeMutation);
} finally {
  console.error = originalError;
  await mounted.unmount();
  await session.close();
}
finish('Chatto GPUix threads live', 13);
