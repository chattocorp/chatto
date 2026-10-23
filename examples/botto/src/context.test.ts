/** Exercise Botto's context selection against the real shared MessagesStore. */
import { expect, test, vi } from 'vitest';
import type { EventConnectionPage } from '@chatto/client/api/roomTimeline';
import { readContext } from './context.ts';
import { contextFixture, row } from './context.fixture.ts';

const location = { roomId: 'room', threadRootId: 'root' };

test('selects recent text, keeps the root, and sees shared edits and retractions', async () => {
  const { server, store, history } = contextFixture();
  const signal = new AbortController().signal;
  const first = await readContext(server, location, 2, signal);
  expect(first.messages.map((message) => message.id)).toEqual(['root', 'one', 'two']);
  expect(first.olderContextOmitted).toBe(true);
  store.captureMessageReconciliation()('two', row('two', 'Edited', 'bot'), false);
  store.applyMessageRetraction('one', '2026-10-23T01:00:00Z');
  const second = await readContext(server, location, 2, signal);
  expect(second.messages.find((message) => message.id === 'one')).toBeUndefined();
  expect(second.messages.find((message) => message.id === 'two')).toMatchObject({
    text: 'Edited',
    fromViewer: true
  });
  expect(history).toHaveBeenCalledTimes(1);
  store.dispose();
});

test('cancellation stops waiting without cancelling another consumer of the shared initial read', async () => {
  let resolve!: (page: EventConnectionPage) => void;
  const { server, store } = contextFixture(
    new Promise((done) => {
      resolve = done;
    })
  );
  const controller = new AbortController();
  const waiting = readContext(server, location, 2, controller.signal);
  controller.abort();
  await expect(waiting).rejects.toThrow();
  resolve({ events: [row('root', 'Root')], hasOlder: false, hasNewer: false });
  expect(await store.waitForLoad()).toBe(true);
  expect(
    (await readContext(server, location, 2, new AbortController().signal)).messages
  ).toHaveLength(1);
  store.dispose();
});

test('a reset rejects old context and the shared recovery read supplies new context', async () => {
  const { server, store, history } = contextFixture();
  await store.waitForLoad();
  store.resetProjectionState();
  expect(await store.waitForLoad()).toBe(false);
  history.mockResolvedValueOnce({
    events: [row('root', 'Recovered')],
    hasOlder: false,
    hasNewer: false
  });
  await store.hydrateRealtimeProjection('cursor', () => true);
  expect(
    (await readContext(server, location, 2, new AbortController().signal)).messages[0]?.text
  ).toBe('Recovered');
  store.clearForAccessRevocation();
  await expect(readContext(server, location, 2, new AbortController().signal)).rejects.toThrow(
    'not ready'
  );
  expect(history).toHaveBeenCalledTimes(2);
  store.dispose();
});

test('loads an extra page only when the selected context needs it', async () => {
  const { server, store, history } = contextFixture(
    Promise.resolve({
      events: [row('root', 'Root'), row('two', 'Two')],
      hasOlder: true,
      hasNewer: false,
      startCursor: 'start',
      endCursor: 'end'
    })
  );
  history.mockResolvedValueOnce({ events: [row('one', 'One')], hasOlder: false, hasNewer: false });
  expect(
    (await readContext(server, location, 2, new AbortController().signal)).messages.map(
      (message) => message.id
    )
  ).toEqual(['root', 'one', 'two']);
  await readContext(server, location, 2, new AbortController().signal);
  expect(history).toHaveBeenCalledTimes(2);
  store.dispose();
});

test('retries failed initial history once and rejects a failed retry', async () => {
  const output = vi.spyOn(console, 'error').mockImplementation(() => {});
  const { server, store, history } = contextFixture(Promise.reject(new Error('Offline')));
  try {
    await expect(readContext(server, location, 2, new AbortController().signal)).rejects.toThrow(
      'not ready'
    );
    expect(history).toHaveBeenCalledTimes(2);
    history.mockResolvedValueOnce({
      events: [row('root', 'Recovered')],
      hasOlder: false,
      hasNewer: false
    });
    expect(
      (await readContext(server, location, 2, new AbortController().signal)).messages[0]?.text
    ).toBe('Recovered');
    expect(history).toHaveBeenCalledTimes(3);
  } finally {
    store.dispose();
    output.mockRestore();
  }
});

test('rejects incomplete context when pagination fails without looping', async () => {
  const output = vi.spyOn(console, 'error').mockImplementation(() => {});
  const { server, store, history } = contextFixture(
    Promise.resolve({
      events: [row('root', 'Root')],
      hasOlder: true,
      hasNewer: false,
      startCursor: 'older'
    })
  );
  history.mockRejectedValueOnce(new Error('Offline'));
  try {
    await expect(readContext(server, location, 2, new AbortController().signal)).rejects.toThrow(
      'not ready'
    );
    expect(history).toHaveBeenCalledTimes(2);
  } finally {
    store.dispose();
    output.mockRestore();
  }
});
