import { afterEach, expect, test, vi } from 'vitest';
import { createChannel } from 'runling';
import { ADDRESSED_MESSAGE_BATCH_MS, MESSAGE_BATCH_MS, messageBatches } from './message-batches.ts';
import type { IncomingMessage } from '../chatto/message.ts';

afterEach(() => vi.useRealTimers());

const message = (id: string, trigger?: IncomingMessage['trigger']): IncomingMessage => ({
  id,
  roomId: 'room',
  threadRootId: 'root',
  authorId: id,
  body: id,
  ...(trigger ? { trigger } : {})
});

test('the two-second addressed window includes the first message and never slides', async () => {
  vi.useFakeTimers();
  const input = createChannel<IncomingMessage>();
  const { batches } = messageBatches(input, new AbortController().signal, [message('first')]);
  const received = vi.fn();
  const first = batches.next().then(received);
  await vi.advanceTimersByTimeAsync(1_000);
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

test.each(['intake', 'follow-up'] as const)(
  'ordinary %s messages wait 45 seconds without extending the window',
  async (trigger) => {
    vi.useFakeTimers();
    const input = createChannel<IncomingMessage>();
    const firstMessage = message('first', trigger);
    const { batches } = messageBatches(input, new AbortController().signal, [firstMessage]);
    const received = vi.fn();
    const first = batches.next().then(received);
    await vi.advanceTimersByTimeAsync(30_000);
    const second = message('second', trigger);
    await input.send(second);
    await vi.advanceTimersByTimeAsync(14_999);
    expect(received).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await first;
    expect(received).toHaveBeenCalledWith({ done: false, value: [firstMessage, second] });
    input.close();
    await batches.next();
    expect(vi.getTimerCount()).toBe(0);
  }
);

test('a direct request brings a pending ordinary batch forward to two seconds', async () => {
  vi.useFakeTimers();
  const input = createChannel<IncomingMessage>();
  const ordinary = message('ordinary', 'follow-up');
  const direct = message('direct');
  const { batches } = messageBatches(input, new AbortController().signal, [ordinary]);
  const received = vi.fn();
  const first = batches.next().then(received);
  await vi.advanceTimersByTimeAsync(10_000);
  await input.send(direct);
  await vi.advanceTimersByTimeAsync(1_999);
  expect(received).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  await first;
  expect(received).toHaveBeenCalledWith({ done: false, value: [ordinary, direct] });
  input.close();
  await batches.next();
});

test('ordinary messages cannot extend a direct request window', async () => {
  vi.useFakeTimers();
  const input = createChannel<IncomingMessage>();
  const direct = message('direct');
  const ordinary = message('ordinary', 'follow-up');
  const { batches } = messageBatches(input, new AbortController().signal, [direct]);
  const first = batches.next();
  await vi.advanceTimersByTimeAsync(1_000);
  await input.send(ordinary);
  await vi.advanceTimersByTimeAsync(1_000);
  expect((await first).value).toEqual([direct, ordinary]);
  input.close();
  await batches.next();
});

test('reactions use the ordinary window', async () => {
  vi.useFakeTimers();
  const input = createChannel<IncomingMessage>();
  const reaction = {
    ...message('reaction'),
    reaction: { emoji: '+1', messageId: 'bot', body: 'Done' }
  };
  const { batches } = messageBatches(input, new AbortController().signal, [reaction]);
  const received = vi.fn();
  const first = batches.next().then(received);
  await vi.advanceTimersByTimeAsync(MESSAGE_BATCH_MS - 1);
  expect(received).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  await first;
  expect(received).toHaveBeenCalledWith({ done: false, value: [reaction] });
  input.close();
  await batches.next();
});

test('the window elapses while the consumer is busy and close drains the final batch', async () => {
  vi.useFakeTimers();
  const input = createChannel<IncomingMessage>();
  const { batches } = messageBatches(input, new AbortController().signal, [message('first')]);
  const first = batches.next();
  await vi.advanceTimersByTimeAsync(ADDRESSED_MESSAGE_BATCH_MS);
  await first;
  await input.send(message('second'));
  await vi.advanceTimersByTimeAsync(1_000);
  await input.send(message('third'));
  await vi.advanceTimersByTimeAsync(1_000);
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
