/** Exercise Botto's context selection against the client's real thread timelines. */
import { afterEach, expect, test, vi } from 'vitest';
import type { EventConnectionPage } from '@chatto/client/api/roomTimeline';
import { RECENT_THREAD_LIMIT } from '@chatto/client/server/roomStores';
import { readContext } from './context.ts';
import { contextFixture, page, row } from './context.fixture.ts';

const location = { roomId: 'room', threadRootId: 'root' };
const read = (server: Parameters<typeof readContext>[0], limit = 2) =>
  readContext(server, location, limit, new AbortController().signal);

afterEach(() => {
  vi.restoreAllMocks();
});

test('selects recent text, keeps the root, and sees realtime edits and deletions', async () => {
  const { server, store, history, rooms } = contextFixture();
  const first = await read(server);
  expect(first.messages.map((message) => message.id)).toEqual(['root', 'one', 'two']);
  expect(first.olderContextOmitted).toBe(true);
  store().captureMessageReconciliation()('two', row('two', 'Edited', 'bot'), false);
  store().applyMessageRetraction('one', '2026-10-23T01:00:00Z');
  store().ingestEvent(row('three', 'Live reply'));
  const second = await read(server);
  expect(second.messages.find((message) => message.id === 'one')).toBeUndefined();
  expect(second.messages.find((message) => message.id === 'two')).toMatchObject({
    text: 'Edited',
    fromViewer: true
  });
  expect(second.messages.at(-1)).toMatchObject({ id: 'three', text: 'Live reply' });
  expect(history).toHaveBeenCalledTimes(1);
  rooms.dispose();
});

test('keeps image-only messages without copying signed asset addresses', async () => {
  const image = row('one', '');
  if (image.event.kind !== 'messagePosted') throw new Error('Expected a message');
  image.event.attachments = [
    {
      id: 'image',
      filename: 'image.png',
      contentType: 'image/png',
      width: 50,
      height: 50,
      assetUrl: { url: '/assets/private?ticket=secret', expiresAt: '2026-10-26T00:00:00Z' }
    }
  ];
  const { server, rooms } = contextFixture(Promise.resolve(page([row('root', 'Root'), image])));
  try {
    const result = await read(server);
    expect(result.messages.at(-1)).toMatchObject({
      id: 'one',
      text: '',
      attachments: [{ id: 'image', contentType: 'image/png' }]
    });
    expect(JSON.stringify(result)).not.toContain('ticket');
  } finally {
    rooms.dispose();
  }
});

test('reports omitted older replies when the first page has older events', async () => {
  const { server, rooms } = contextFixture(Promise.resolve(page([row('root', 'Root')], true)));
  expect((await read(server)).olderContextOmitted).toBe(true);
  rooms.dispose();
});

test('gets the thread again when the client disposes it during the wait', async () => {
  const { server, rooms, history } = contextFixture(new Promise(() => {}));
  const waiting = read(server);
  history.mockResolvedValue(page([row('root', 'Root again')]));
  // Using more threads than the client keeps disposes the waiting one.
  for (let index = 0; index < RECENT_THREAD_LIMIT; index++) rooms.thread('room', `other-${index}`);
  expect((await waiting).messages[0]?.text).toBe('Root again');
  rooms.dispose();
});

test('cancellation stops waiting without cancelling the shared read', async () => {
  let resolve!: (page: EventConnectionPage) => void;
  const { server, store, rooms } = contextFixture(
    new Promise((done) => {
      resolve = done;
    })
  );
  const controller = new AbortController();
  const waiting = readContext(server, location, 2, controller.signal);
  controller.abort(new Error('Stopped'));
  await expect(waiting).rejects.toThrow('Stopped');
  resolve(page([row('root', 'Root')]));
  await vi.waitFor(() => expect(store().isCurrent).toBe(true));
  expect((await read(server)).messages).toHaveLength(1);
  rooms.dispose();
});

test('retries a failed load once and rejects a failed retry', async () => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  const { server, history, rooms } = contextFixture(Promise.reject(new Error('Offline')));
  await expect(read(server)).rejects.toThrow('not ready');
  expect(history).toHaveBeenCalledTimes(2);
  history.mockResolvedValueOnce(page([row('root', 'Recovered')]));
  expect((await read(server)).messages[0]?.text).toBe('Recovered');
  expect(history).toHaveBeenCalledTimes(3);
  rooms.dispose();
});

test('waits for a reconnect reload and rejects after access loss', async () => {
  const { server, store, history, rooms } = contextFixture();
  await read(server);
  store().resetProjectionState();
  history.mockResolvedValueOnce(page([row('root', 'Reloaded')]));
  const waiting = read(server);
  await store().hydrateRealtimeProjection('cursor', () => true);
  expect((await waiting).messages[0]?.text).toBe('Reloaded');
  rooms.clearMessageAccess('room');
  await expect(read(server)).rejects.toThrow('not ready');
  rooms.dispose();
});

test('waits while Chatto reconnects', async () => {
  const { server, history, rooms } = contextFixture();
  server.status = 'connecting';
  const waiting = read(server);
  await Promise.resolve();
  expect(history).not.toHaveBeenCalled();

  server.status = 'connected';
  expect((await waiting).messages.map((message) => message.id)).toEqual(['root', 'one', 'two']);
  rooms.dispose();
});

test('stops waiting for a reconnect when the turn ends', async () => {
  const { server, history, rooms } = contextFixture();
  server.status = 'disconnected';
  const controller = new AbortController();
  const waiting = readContext(server, location, 2, controller.signal);
  controller.abort(new Error('Turn timed out'));
  await expect(waiting).rejects.toThrow('Turn timed out');
  expect(history).not.toHaveBeenCalled();
  rooms.dispose();
});

test('does not mark rows without an author as the viewer’s without an account ID', async () => {
  const { server, rooms } = contextFixture(
    Promise.resolve(page([row('root', 'Root'), row('anonymous', 'Hi', null as never)]))
  );
  const anonymousServer = { ...server, accountId: null };
  const context = await read(anonymousServer);
  expect(context.messages.find((message) => message.id === 'anonymous')?.fromViewer).toBe(false);
  rooms.dispose();
});
