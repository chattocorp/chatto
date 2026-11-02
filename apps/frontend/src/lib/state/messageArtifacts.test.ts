import { expect, it } from 'vitest';
import { signal } from '@chatto/client/reactivity';
import { messageEvent } from '../../routes/chat/[serverId]/[roomId]/messageEventFixture';
import { messageArtifactOwner } from './messageArtifacts';

it('keeps owners for unchanged bodies and drops them at timeline privacy boundaries', () => {
  const event = messageEvent();
  const events = signal([event]);
  const disposed = signal(false);
  const store = {
    get events() {
      return events.get();
    },
    get isDisposed() {
      return disposed.get();
    }
  };
  const body = 'Hello from this message';
  const first = messageArtifactOwner(store, event.id, body);
  expect(first).toBeDefined();
  events.set([messageEvent({ replyCount: 2 })]);
  expect(messageArtifactOwner(store, event.id, body)).toBe(first);
  events.set([messageEvent({ body: 'Edited' })]);
  expect(messageArtifactOwner(store, event.id, body)).toBeUndefined();
  expect(messageArtifactOwner(store, event.id, 'Edited')).not.toBe(first);
  events.set([event]);
  const restored = messageArtifactOwner(store, event.id, body);
  expect(restored).not.toBe(first);
  // Removal/reset must discard the identity even if no row reads the cache in between.
  events.set([]);
  events.set([event]);
  const reloaded = messageArtifactOwner(store, event.id, body);
  expect(reloaded).not.toBe(restored);
  const deleted = messageEvent();
  if (deleted.event.kind !== 'messagePosted') throw new Error('Expected a message');
  deleted.event.deletedAt = new Date().toISOString();
  events.set([deleted]);
  expect(messageArtifactOwner(store, event.id, body)).toBeUndefined();
  events.set([event]);
  expect(messageArtifactOwner(store, event.id, body)).not.toBe(reloaded);
  const otherStore = { events: [event], isDisposed: false };
  expect(messageArtifactOwner(otherStore, event.id, body)).not.toBe(
    messageArtifactOwner(store, event.id, body)
  );
  disposed.set(true);
  expect(messageArtifactOwner(store, event.id, body)).toBeUndefined();
});
