import { afterEach, expect, test, vi } from 'vitest';
import { createChannel } from 'runling';
import { messageBatches } from './message-batches.ts';
import type { IncomingMessage } from '../chatto/message.ts';

afterEach(() => vi.useRealTimers());

const message = (id: string): IncomingMessage => ({
  id,
  roomId: 'room',
  threadRootId: 'root',
  authorId: id,
  body: id
});

test('the default three-second window includes the first message and never slides', async () => {
  vi.useFakeTimers();
  const input = createChannel<IncomingMessage>();
  const { batches } = messageBatches(input, new AbortController().signal, [message('first')]);
  const received = vi.fn();
  const first = batches.next().then(received);
  await vi.advanceTimersByTimeAsync(2_000);
  await input.send(message('second'));
  await vi.advanceTimersByTimeAsync(999);
  expect(received).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  await first;
  expect(received).toHaveBeenCalledWith({
    done: false,
    value: [message('first'), message('second')]
  });
  input.close();
  expect((await batches.next()).done).toBe(true);
  expect(vi.getTimerCount()).toBe(0);
});

test('the window elapses while the consumer is busy and close drains the final batch', async () => {
  vi.useFakeTimers();
  const input = createChannel<IncomingMessage>();
  const { batches } = messageBatches(input, new AbortController().signal, [message('first')]);
  const first = batches.next();
  await vi.advanceTimersByTimeAsync(3_000);
  await first;
  await input.send(message('second'));
  await vi.advanceTimersByTimeAsync(1_000);
  await input.send(message('third'));
  await vi.advanceTimersByTimeAsync(2_000);
  input.close();
  expect((await batches.next()).value).toEqual([message('second'), message('third')]);
  expect((await batches.next()).done).toBe(true);
  expect(vi.getTimerCount()).toBe(0);
});

test('cancellation during a window discards queued messages and removes the timer', async () => {
  vi.useFakeTimers();
  const controller = new AbortController();
  const input = createChannel<IncomingMessage>({ signal: controller.signal });
  const { batches } = messageBatches(input, controller.signal, [message('first')]);
  const reason = new Error('stop');
  const rejected = expect(batches.next()).rejects.toBe(reason);
  await vi.advanceTimersByTimeAsync(1_000);
  controller.abort(reason);
  await rejected;
  expect(vi.getTimerCount()).toBe(0);
});
