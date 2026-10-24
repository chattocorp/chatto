/** Exercise Botto's context selection against the client's real thread timelines. */
import { afterEach, expect, test, vi } from 'vitest';
import type { EventConnectionPage } from '@chatto/client/api/roomTimeline';
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

test('reports omitted older replies when the first page has older events', async () => {
  const { server, rooms } = contextFixture(Promise.resolve(page([row('root', 'Root')], true)));
  expect((await read(server)).olderContextOmitted).toBe(true);
  rooms.dispose();
});

test('releases the thread after reading so the client can dispose it later', async () => {
  const { server, rooms } = contextFixture();
  const retain = vi.spyOn(rooms, 'retainThread');
  const release = vi.spyOn(rooms, 'releaseThread');
  await read(server);
  expect(retain).toHaveBeenCalledOnce();
  expect(release).toHaveBeenCalledOnce();
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

test('rejects while Chatto is not connected', async () => {
  const { server, history, rooms } = contextFixture();
  server.status = 'connecting';
  await expect(read(server)).rejects.toThrow('not connected');
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
