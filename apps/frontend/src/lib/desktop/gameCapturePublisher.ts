// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import type { GameCapturePublisherRequest } from './gameCapture';

const publisherMessageType = 'chatto:game-capture:publisher';
const publisherStartTimeoutMs = 20_000;
const publisherStopTimeoutMs = 5_000;

type PublisherMessage =
  | { kind: 'started'; width: number; height: number; frameRate: number }
  | { kind: 'error'; message?: string }
  | { kind: 'ended' };

/** Control handle for a native helper that publishes game media to LiveKit. */
export class GameCapturePublisherSession {
  onEnded: ((error?: Error) => void) | null = null;

  readonly #port: MessagePort;
  #finished = false;
  #stopPromise: Promise<void> | null = null;
  #resolveStop: (() => void) | null = null;
  #rejectStop: ((error: Error) => void) | null = null;
  #stopTimeout: number | null = null;

  private constructor(port: MessagePort) {
    this.#port = port;
    port.onmessage = (event: MessageEvent<PublisherMessage>) => {
      const message = event.data;
      if (message.kind === 'error') {
        this.finish(new Error(message.message || 'Native game publishing stopped unexpectedly.'));
      } else if (message.kind === 'ended') {
        this.finish();
      }
    };
    port.onmessageerror = () =>
      this.finish(new Error('The desktop game-publisher control channel failed.'));
    port.start();
  }

  /** Start the native publisher and resolve only after LiveKit accepted its tracks. */
  static async start(request: GameCapturePublisherRequest): Promise<GameCapturePublisherSession> {
    const port = await requestPublisherPort(request);
    await waitForPublisherStarted(port);
    return new GameCapturePublisherSession(port);
  }

  /** Ask Desktop to stop publishing and wait for the helper to exit. */
  stop(): Promise<void> {
    if (this.#stopPromise) return this.#stopPromise;
    if (this.#finished) return Promise.resolve();

    this.#stopPromise = new Promise<void>((resolve, reject) => {
      this.#resolveStop = resolve;
      this.#rejectStop = reject;
    });
    this.#port.postMessage({ kind: 'stop' });
    this.#stopTimeout = window.setTimeout(() => {
      this.finish(new Error('The native game publisher did not stop in time.'));
    }, publisherStopTimeoutMs);
    return this.#stopPromise;
  }

  private finish(error?: Error): void {
    if (this.#finished) return;
    this.#finished = true;
    if (this.#stopTimeout !== null) window.clearTimeout(this.#stopTimeout);
    this.#port.close();
    if (this.#stopPromise) {
      if (error) this.#rejectStop?.(error);
      else this.#resolveStop?.();
    } else {
      this.onEnded?.(error);
    }
    this.#resolveStop = null;
    this.#rejectStop = null;
  }
}

function requestPublisherPort(request: GameCapturePublisherRequest): Promise<MessagePort> {
  const bridge = window.chattoDesktop?.gameCapture;
  if (!bridge) return Promise.reject(new Error('Native game capture is unavailable.'));

  return new Promise((resolve, reject) => {
    let requestId = '';
    const timeout = window.setTimeout(() => {
      cleanup();
      reject(new Error('Native game publishing did not start in time.'));
    }, publisherStartTimeoutMs);
    const receivePort = (event: MessageEvent) => {
      if (
        event.source !== window ||
        event.data?.type !== publisherMessageType ||
        event.data?.requestId !== requestId ||
        event.ports.length !== 1
      ) {
        return;
      }
      cleanup();
      resolve(event.ports[0]);
    };
    const cleanup = () => {
      window.clearTimeout(timeout);
      window.removeEventListener('message', receivePort);
    };
    window.addEventListener('message', receivePort);
    try {
      requestId = bridge.startPublisher(request);
    } catch (error) {
      cleanup();
      reject(error);
    }
  });
}

function waitForPublisherStarted(port: MessagePort): Promise<void> {
  return new Promise((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      cleanup();
      port.close();
      reject(new Error('Native game publishing did not connect to LiveKit in time.'));
    }, publisherStartTimeoutMs);
    port.onmessage = (event: MessageEvent<PublisherMessage>) => {
      const message = event.data;
      if (message.kind === 'started') {
        cleanup();
        resolve();
      } else if (message.kind === 'error') {
        cleanup();
        port.close();
        reject(new Error(message.message || 'Native game publishing could not start.'));
      } else if (message.kind === 'ended') {
        cleanup();
        port.close();
        reject(new Error('Native game publishing stopped before it started.'));
      }
    };
    port.start();
    const cleanup = () => window.clearTimeout(timeout);
  });
}
