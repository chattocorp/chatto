// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import type { NativeScreenSharePublisherRequest } from './nativeScreenShare';

const publisherMessageType = 'chatto:game-capture:publisher';
const publisherStartTimeoutMs = 20_000;
const publisherStopTimeoutMs = 5_000;

type PublisherMessage =
  | {
      kind: 'started';
      width: number;
      height: number;
      frameRate: number;
      localPreviewAvailable?: boolean;
    }
  | { kind: 'preview-frame'; timestampUs: number; keyFrame: boolean; data: Uint8Array }
  | {
      kind: 'metrics';
      submittedFrames: number;
      publishedFrames: number;
      droppedFrames: number;
      captureFps: number;
      publishFps: number;
      averageReadbackMs: number;
      averageScaleMs: number;
      averagePublishMs: number;
      averageHardwareEncodeMs: number;
      hardwareEncoderImplementation: string;
      requestedEncoderBitrate: number;
      appliedEncoderBitrate: number;
      actualHardwareBitrate: number;
      encoderRateControlMode: number;
      requestedEncoderFps: number;
      hardwareEncodedFrames: number;
      hardwareEncodedBytes: number;
      hardwareKeyFrames: number;
      hardwareEncodedWidth: number;
      hardwareEncodedHeight: number;
      encoderResolutionChanges: number;
      lastPublishMs: number;
      sourceWidth: number;
      sourceHeight: number;
      dimensionChanges: number;
      captureBackend: 'wgc-window' | 'wgc-monitor' | 'dxgi-display';
      rtcStatsAvailable: boolean;
      outboundStreams: number;
      activeOutboundStreams: number;
      minimumActiveOutboundFps: number;
      maximumActiveOutboundFps: number;
      framesEncoded: number;
      framesSent: number;
      bytesSent: number;
      retransmittedPacketsSent: number;
      retransmittedBytesSent: number;
      nackCount: number;
      pliCount: number;
      targetBitrate: number;
      averageEncodeMs: number;
      encodedWidth: number;
      encodedHeight: number;
      averageQp: number;
      encoderImplementation: string;
      cpuLimitedStreams: number;
      bandwidthLimitedStreams: number;
      powerEfficientStreams: number;
      remoteInboundStatsAvailable: boolean;
      remotePacketsLost: number;
      remoteJitterSeconds: number;
      remoteFractionLost: number;
      remoteRoundTripTimeMs: number;
      candidatePairStatsAvailable: boolean;
      availableOutgoingBitrate: number;
      currentRoundTripTimeMs: number;
      packetsDiscardedOnSend: number;
      bytesDiscardedOnSend: number;
    }
  | { kind: 'stopping' }
  | { kind: 'error'; message?: string }
  | { kind: 'ended' };

export type NativeScreenSharePreviewFrame = {
  timestampUs: number;
  keyFrame: boolean;
  data: Uint8Array;
};

export type NativeScreenSharePreview = {
  width: number;
  height: number;
  frameRate: number;
  subscribe(listener: (frame: NativeScreenSharePreviewFrame) => void): () => void;
};

/** Control handle for a native helper that publishes screen-share media to LiveKit. */
export class NativeScreenSharePublisherSession {
  onEnded: ((error?: Error) => void) | null = null;

  readonly #port: MessagePort;
  readonly preview: NativeScreenSharePreview | null;
  readonly #previewListeners = new Set<(frame: NativeScreenSharePreviewFrame) => void>();
  #finished = false;
  #stopPromise: Promise<void> | null = null;
  #resolveStop: (() => void) | null = null;
  #rejectStop: ((error: Error) => void) | null = null;
  #stopTimeout: number | null = null;

  private constructor(
    port: MessagePort,
    started: Extract<PublisherMessage, { kind: 'started' }>
  ) {
    this.#port = port;
    this.preview = started.localPreviewAvailable
      ? {
          width: started.width,
          height: started.height,
          frameRate: started.frameRate,
          subscribe: (listener) => {
            this.#previewListeners.add(listener);
            return () => this.#previewListeners.delete(listener);
          }
        }
      : null;
    port.onmessage = (event: MessageEvent<PublisherMessage>) => {
      const message = event.data;
      if (message.kind === 'error') {
        this.finish(new Error(message.message || 'Native screen sharing stopped unexpectedly.'));
      } else if (message.kind === 'ended') {
        this.finish();
      } else if (message.kind === 'metrics') {
        console.info('[Chatto Desktop] Native screen-share publisher metrics', message);
      } else if (message.kind === 'stopping') {
        console.info('[Chatto Desktop] Native screen-share publisher stopping');
      } else if (message.kind === 'preview-frame') {
        for (const listener of this.#previewListeners) listener(message);
      }
    };
    port.onmessageerror = () =>
      this.finish(new Error('The desktop screen-share control channel failed.'));
    port.start();
  }

  /** Start the native publisher and resolve only after LiveKit accepted its tracks. */
  static async start(
    request: NativeScreenSharePublisherRequest
  ): Promise<NativeScreenSharePublisherSession> {
    const port = await requestPublisherPort(request);
    const started = await waitForPublisherStarted(port);
    return new NativeScreenSharePublisherSession(port, started);
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
      this.finish(new Error('The native screen-share publisher did not stop in time.'));
    }, publisherStopTimeoutMs);
    return this.#stopPromise;
  }

  private finish(error?: Error): void {
    if (this.#finished) return;
    this.#finished = true;
    if (this.#stopTimeout !== null) window.clearTimeout(this.#stopTimeout);
    this.#port.close();
    this.#previewListeners.clear();
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

function requestPublisherPort(request: NativeScreenSharePublisherRequest): Promise<MessagePort> {
  const bridge = window.chattoDesktop?.screenShare;
  if (typeof bridge?.startPublisher !== 'function') {
    return Promise.reject(new Error('Native screen sharing is unavailable.'));
  }

  return new Promise((resolve, reject) => {
    let requestId = '';
    const timeout = window.setTimeout(() => {
      cleanup();
      reject(new Error('Native screen sharing did not start in time.'));
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

function waitForPublisherStarted(
  port: MessagePort
): Promise<Extract<PublisherMessage, { kind: 'started' }>> {
  return new Promise((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      cleanup();
      port.close();
      reject(new Error('Native screen sharing did not connect to LiveKit in time.'));
    }, publisherStartTimeoutMs);
    port.onmessage = (event: MessageEvent<PublisherMessage>) => {
      const message = event.data;
      if (message.kind === 'started') {
        cleanup();
        resolve(message);
      } else if (message.kind === 'error') {
        cleanup();
        port.close();
        reject(new Error(message.message || 'Native screen sharing could not start.'));
      } else if (message.kind === 'ended') {
        cleanup();
        port.close();
        reject(new Error('Native screen sharing stopped before it started.'));
      }
    };
    port.start();
    const cleanup = () => window.clearTimeout(timeout);
  });
}
