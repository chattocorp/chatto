export class ChannelClosedError extends Error {
  override readonly name = "ChannelClosedError";

  constructor() {
    super("Channel is closed");
  }
}

export class ChannelFullError extends Error {
  override readonly name = "ChannelFullError";

  constructor() {
    super("Channel buffer is full");
  }
}

export interface Channel<T> extends AsyncIterable<T> {
  /** Queue a value. Does not wait for processing or free buffer space. */
  send(value: T): Promise<void>;

  /** Stop sends and let the consumer drain queued values. Idempotent. */
  close(): void;

  /** Discard queued values and reject reads and sends. Idempotent. */
  fail(reason: unknown): void;
}

/** Bounded FIFO with one iterator and at most one pending read. */
export function createChannel<T>({
  capacity = 64,
  signal,
}: { capacity?: number; signal?: AbortSignal } = {}): Channel<T> {
  if (!Number.isSafeInteger(capacity) || capacity < 1) {
    throw new RangeError("Channel capacity must be a positive safe integer");
  }

  // Keep failure separate from state: even undefined can be a rejection reason.
  let state: "open" | "closed" | "failed" = "open";
  let failure: unknown;
  const queue: T[] = [];

  let reader:
    | {
        resolve: (result: IteratorResult<T>) => void;
        reject: (error: unknown) => void;
      }
    | undefined;
  let claimed = false;

  const detach = () => signal?.removeEventListener("abort", abort);
  const done = (): IteratorResult<T> => ({ done: true, value: undefined });
  const abort = () => channel.fail(signal!.reason);

  const channel: Channel<T> = {
    async send(value) {
      if (state === "failed") throw failure;
      if (state === "closed") throw new ChannelClosedError();

      if (reader) {
        // A waiting consumer receives the value without using buffer space.
        const waiting = reader;
        reader = undefined;
        waiting.resolve({ done: false, value });
      } else {
        if (queue.length >= capacity) throw new ChannelFullError();

        queue.push(value);
      }
    },

    close() {
      if (state !== "open") return;

      // Graceful close preserves queued values for the consumer to drain.
      state = "closed";
      detach();

      reader?.resolve(done());
      reader = undefined;
    },

    fail(reason) {
      if (state !== "open") return;

      // Failure discards pending data. The first terminal transition wins.
      state = "failed";
      failure = reason;
      queue.length = 0;
      detach();

      reader?.reject(reason);
      reader = undefined;
    },

    [Symbol.asyncIterator]() {
      // Sharing a channel between readers must not silently split its messages.
      if (claimed) throw new Error("Channel already has a consumer");

      claimed = true;
      let returned = false;

      return {
        next(): Promise<IteratorResult<T>> {
          if (returned) return Promise.resolve(done());
          if (state === "failed") return Promise.reject(failure);

          // Read buffered values before checking for a graceful close.
          if (queue.length) {
            return Promise.resolve({ done: false, value: queue.shift()! });
          }

          if (state === "closed") return Promise.resolve(done());
          if (reader) {
            return Promise.reject(
              new Error("Channel already has a pending read"),
            );
          }

          return new Promise((resolve, reject) => {
            reader = { resolve, reject };
          });
        },

        async return() {
          // A for-await loop calls return when its consumer leaves early.
          returned = true;
          queue.length = 0;
          channel.close();

          return done();
        },
      };
    },
  };

  if (signal?.aborted) {
    abort();
  } else {
    signal?.addEventListener("abort", abort, { once: true });
  }

  return channel;
}
