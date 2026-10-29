/** Reopen real state files to exercise process restart boundaries. */
import { mkdirSync, mkdtempSync, readdirSync, rmSync, statSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import type { AddressedMessage } from '@chatto/client';
import { Delivery } from './delivery.ts';
import { Inbox } from './inbox.ts';

const directories: string[] = [];
function directory(): string {
  const root = resolve('../../.context');
  mkdirSync(root, { recursive: true });
  const path = mkdtempSync(join(root, 'botto-delivery-'));
  directories.push(path);
  return path;
}
afterEach(() => {
  vi.useRealTimers();
  for (const path of directories.splice(0)) rmSync(path, { recursive: true, force: true });
});

const message: AddressedMessage = {
  id: 'one',
  roomId: 'room',
  authorId: 'human',
  body: 'Hello',
  reasons: ['mention']
};

test('restart restores queued work, isolates identities, and retains the first boundary', () => {
  const path = directory();
  const first = new Delivery(path, ['server', 'bot'], 100);
  first.accept(message);
  first.accept(message);
  const restarted = new Delivery(path, ['server', 'bot'], 200);
  expect(restarted.since).toBe(100);
  expect(restarted.queued).toEqual([message]);
  expect(new Delivery(path, ['server', 'other'], 200).queued).toEqual([]);
  for (const name of readdirSync(path)) expect(statSync(join(path, name)).mode & 0o777).toBe(0o600);
});

test('interrupted external effects remain visible but are not repeated', () => {
  const path = directory();
  const first = new Delivery(path, ['bot']);
  first.accept(message);
  first.start([message]);
  const restarted = new Delivery(path, ['bot']);
  expect(restarted.uncertain).toBe(1);
  expect(restarted.queued).toEqual([]);
  restarted.accept(message);
  expect(restarted.uncertain).toBe(1);
});

test('reaction metadata survives restart and duplicate events are processed once', () => {
  const path = directory();
  const reaction = {
    ...message,
    id: 'reaction-event',
    body: '',
    reasons: [],
    reaction: { messageId: 'answer', emoji: 'thinking_face' }
  };
  const first = new Delivery(path, ['bot']);
  first.accept(reaction);
  const restarted = new Delivery(path, ['bot']);
  restarted.accept(reaction);
  expect(restarted.queued).toEqual([reaction]);
  restarted.complete([reaction]);
  expect(new Delivery(path, ['bot']).has(reaction.id)).toBe(true);
});

test('completed work stays deduplicated across restarts', () => {
  const path = directory();
  const first = new Delivery(path, ['bot']);
  first.accept(message);
  first.start([message]);
  first.complete([message]);
  const restarted = new Delivery(path, ['bot']);
  restarted.accept(message);
  expect(restarted.has(message.id)).toBe(true);
  expect(restarted.queued).toEqual([]);
  expect(restarted.uncertain).toBe(0);
});

test('a failed save cannot acknowledge work in memory that is absent on disk', () => {
  const path = directory();
  const delivery = new Delivery(path, ['bot']);
  const name = readdirSync(path)[0]!;
  mkdirSync(join(path, `${name}.new`));
  expect(() => delivery.accept(message)).toThrow();
  expect(delivery.has(message.id)).toBe(false);
  expect(new Delivery(path, ['bot']).queued).toEqual([]);
});

test('failed state transitions preserve queued and running work in memory and on disk', () => {
  const path = directory();
  const delivery = new Delivery(path, ['bot']);
  delivery.accept(message);
  const temporary = join(path, `${readdirSync(path)[0]!}.new`);
  mkdirSync(temporary);
  expect(() => delivery.start([message])).toThrow();
  expect(delivery.queued).toEqual([message]);
  expect(new Delivery(path, ['bot']).queued).toEqual([message]);
  rmSync(temporary, { recursive: true });
  delivery.start([message]);
  mkdirSync(temporary);
  expect(() => delivery.complete([message])).toThrow();
  expect(delivery.uncertain).toBe(1);
  expect(new Delivery(path, ['bot']).uncertain).toBe(1);
});

test('shutdown during debounce and queue overflow do not lose durable work', async () => {
  vi.useFakeTimers();
  const path = directory();
  const first = new Delivery(path, ['bot']);
  const second = { ...message, id: 'two' };
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn(), {
    debounceMs: 2000,
    passiveDebounceMs: 5000,
    maxPendingMessages: 1,
    maxThreads: 1
  });
  first.accept(message);
  first.accept(second);
  expect(inbox.accept(message)).toBe('accepted');
  expect(inbox.accept(second)).toBe('full');
  await inbox.close();
  const restarted = new Delivery(path, ['bot']);
  expect(restarted.queued).toEqual([message, second]);
  expect(turn).not.toHaveBeenCalled();
});
