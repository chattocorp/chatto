import { afterEach, describe, expect, it, vi } from 'vitest';
import { GameCapturePublisherSession } from './gameCapturePublisher';

afterEach(() => {
  delete window.chattoDesktop;
});

describe('GameCapturePublisherSession', () => {
  it('waits for the native publisher to acknowledge stop', async () => {
    const hostChannel = new MessageChannel();
    const stopped = vi.fn();
    hostChannel.port1.onmessage = (event) => {
      if (event.data?.kind !== 'stop') return;
      stopped();
      hostChannel.port1.postMessage({ kind: 'ended' });
    };
    hostChannel.port1.start();

    window.chattoDesktop = {
      gameCapture: {
        listSources: async () => ({ protocolVersion: 1, sources: [] }),
        startPublisher: () => {
          queueMicrotask(() => {
            window.postMessage(
              { type: 'chatto:game-capture:publisher', requestId: 'publisher-1' },
              window.origin,
              [hostChannel.port2]
            );
            hostChannel.port1.postMessage({
              kind: 'started',
              width: 1920,
              height: 1080,
              frameRate: 60
            });
          });
          return 'publisher-1';
        }
      }
    };

    const session = await GameCapturePublisherSession.start({
      sourceId: 'window:42',
      livekitUrl: 'wss://livekit.example.test',
      token: 'publisher-token',
      e2eeKey: 'shared-e2ee-key'
    });

    await session.stop();

    expect(stopped).toHaveBeenCalledOnce();
  });
});
