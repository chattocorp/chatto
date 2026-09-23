import { expect, test, vi } from "vitest";
import {
  ChannelClosedError,
  ChannelFullError,
  createChannel,
} from "./channel.ts";

async function collect<T>(source: AsyncIterable<T>) {
  const values: T[] = [];
  for await (const value of source) values.push(value);
  return values;
}

test("queues FIFO values before a reader starts, including undefined", async () => {
  const channel = createChannel<number | undefined>({ capacity: 3 });
  await Promise.all([
    channel.send(1),
    channel.send(undefined),
    channel.send(3),
  ]);
  channel.close();

  expect(await collect(channel)).toEqual([1, undefined, 3]);
  await expect(channel.send(4)).rejects.toBeInstanceOf(ChannelClosedError);
  channel.close();
});

test("rejects overflow without dropping or queueing the rejected value", async () => {
  const channel = createChannel<number>({ capacity: 1 });
  await channel.send(1);
  await expect(channel.send(2)).rejects.toBeInstanceOf(ChannelFullError);
  const reader = channel[Symbol.asyncIterator]();

  expect(await reader.next()).toEqual({ value: 1, done: false });
  await channel.send(3);

  expect(await reader.next()).toEqual({ value: 3, done: false });
  await reader.return?.();
});

test("delivers to a pending reader and releases it on close", async () => {
  const channel = createChannel<string>();
  const reader = channel[Symbol.asyncIterator]();

  const first = reader.next();
  await channel.send("hello");

  expect(await first).toEqual({ done: false, value: "hello" });

  const last = reader.next();
  channel.close();

  expect(await last).toEqual({ done: true, value: undefined });
  expect(await reader.next()).toEqual({ done: true, value: undefined });
});

test("enforces a single consumer and one pending next call", async () => {
  const channel = createChannel<number>();
  const reader = channel[Symbol.asyncIterator]();

  expect(() => channel[Symbol.asyncIterator]()).toThrow(
    "already has a consumer",
  );
  const waiting = reader.next();
  await expect(reader.next()).rejects.toThrow("pending read");
  channel.close();
  await waiting;
});

test.each([new Error("broken"), undefined, null, "failure"])(
  "failure clears queued data and is retained: %s",
  async (reason) => {
    const channel = createChannel<number>();
    await channel.send(1);
    channel.fail(reason);
    channel.close();
    channel.fail(new Error("later"));
    await expect(channel.send(2)).rejects.toBe(reason);
    await expect(channel[Symbol.asyncIterator]().next()).rejects.toBe(reason);
  },
);

test("failure releases an outstanding reader", async () => {
  const channel = createChannel<number>();
  const pending = channel[Symbol.asyncIterator]().next();
  const failure = new Error("failed");
  channel.fail(failure);
  await expect(pending).rejects.toBe(failure);
});

test("breaking iteration discards the buffer and prevents future sends", async () => {
  const channel = createChannel<number>();
  await channel.send(1);
  await channel.send(2);
  for await (const value of channel) {
    expect(value).toBe(1);
    break;
  }
  await expect(channel.send(3)).rejects.toBeInstanceOf(ChannelClosedError);
});

test("return releases a pending read and is idempotent", async () => {
  const channel = createChannel<number>();
  const reader = channel[Symbol.asyncIterator]();
  const waiting = reader.next();
  await reader.return?.();

  expect((await waiting).done).toBe(true);
  expect((await reader.next()).done).toBe(true);
  await reader.return?.();
});

test.each([true, false])(
  "abort before/after construction releases operations: %s",
  async (before) => {
    const controller = new AbortController();
    const reason = new Error("cancelled");
    if (before) controller.abort(reason);
    const channel = createChannel<number>({ signal: controller.signal });
    const pending = channel[Symbol.asyncIterator]().next();
    controller.abort(reason);
    await expect(pending).rejects.toBe(reason);
    await expect(channel.send(1)).rejects.toBe(reason);
  },
);

test("terminal operations remove the abort listener and preserve a graceful close", async () => {
  const controller = new AbortController();
  const remove = vi.spyOn(controller.signal, "removeEventListener");
  const channel = createChannel<number>({ signal: controller.signal });
  await channel.send(1);
  channel.close();

  expect(remove).toHaveBeenCalledOnce();
  controller.abort();

  expect(await collect(channel)).toEqual([1]);
});

test.each([0, -1, 1.5, Infinity, NaN, Number.MAX_SAFE_INTEGER + 1])(
  "rejects invalid capacity %s",
  (capacity) => {
    expect(() => createChannel({ capacity })).toThrow(RangeError);
  },
);

test("many interleaved sends and reads retain order", async () => {
  const channel = createChannel<number>({ capacity: 4 });
  const values = collect(channel);
  for (let i = 0; i < 1000; i++) await channel.send(i);
  channel.close();

  expect(await values).toEqual(Array.from({ length: 1000 }, (_, i) => i));
});
