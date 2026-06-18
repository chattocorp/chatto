import { afterEach, describe, expect, it, vi } from 'vitest';
import { onNotificationClick } from './pushNotifications';

function deferred<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });

  return { promise, resolve, reject };
}

function stubServiceWorker() {
  const listeners = new Set<(event: MessageEvent) => void>();

  vi.stubGlobal('navigator', {
    serviceWorker: {
      addEventListener: vi.fn((type: string, listener: (event: MessageEvent) => void) => {
        if (type === 'message') listeners.add(listener);
      }),
      removeEventListener: vi.fn((type: string, listener: (event: MessageEvent) => void) => {
        if (type === 'message') listeners.delete(listener);
      })
    }
  });

  return {
    dispatchMessage(event: Pick<MessageEvent, 'data' | 'ports'>) {
      for (const listener of listeners) {
        listener(event as MessageEvent);
      }
    },
    listenerCount() {
      return listeners.size;
    }
  };
}

describe('onNotificationClick', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('acknowledges after the notification callback completes', async () => {
    const serviceWorker = stubServiceWorker();
    const navigation = deferred();
    const callback = vi.fn(() => navigation.promise);
    const responsePort = { postMessage: vi.fn() };
    const stop = onNotificationClick(callback);

    serviceWorker.dispatchMessage({
      data: {
        type: 'notification-click',
        url: 'https://chatto.example/chat/-/room-1'
      },
      ports: [responsePort as unknown as MessagePort]
    });

    await Promise.resolve();
    expect(callback).toHaveBeenCalledWith('https://chatto.example/chat/-/room-1');
    expect(responsePort.postMessage).not.toHaveBeenCalled();

    navigation.resolve();
    await navigation.promise;
    await Promise.resolve();

    expect(responsePort.postMessage).toHaveBeenCalledWith({ type: 'notification-click-ack' });

    stop();
    expect(serviceWorker.listenerCount()).toBe(0);
  });

  it('does not acknowledge when the callback rejects', async () => {
    const serviceWorker = stubServiceWorker();
    const callback = vi.fn(async () => {
      throw new Error('navigation failed');
    });
    const responsePort = { postMessage: vi.fn() };
    onNotificationClick(callback);

    serviceWorker.dispatchMessage({
      data: {
        type: 'notification-click',
        url: 'https://chatto.example/chat/-/room-1'
      },
      ports: [responsePort as unknown as MessagePort]
    });

    await Promise.resolve();
    await Promise.resolve();

    expect(callback).toHaveBeenCalledOnce();
    expect(responsePort.postMessage).not.toHaveBeenCalled();
  });
});
