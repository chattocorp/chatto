import { afterEach, describe, expect, it, vi } from 'vitest';
import { NativeScreenSharePublisherSession } from './nativeScreenSharePublisher';

afterEach(() => {
  delete window.chattoDesktop;
});

describe('NativeScreenSharePublisherSession', () => {
  it('waits for the native publisher to acknowledge stop', async () => {
    const hostChannel = new MessageChannel();
    const stopped = vi.fn();
    hostChannel.port1.onmessage = (event) => {
      if (event.data?.kind !== 'stop') return;
      stopped();
      hostChannel.port1.postMessage({ kind: 'stopping' });
      hostChannel.port1.postMessage({ kind: 'ended' });
    };
    hostChannel.port1.start();

    window.chattoDesktop = {
      screenShare: {
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
              frameRate: 60,
              localPreviewAvailable: true
            });
          });
          return 'publisher-1';
        }
      }
    };

    const session = await NativeScreenSharePublisherSession.start({
      sourceId: 'window:42',
      livekitUrl: 'wss://livekit.example.test',
      token: 'publisher-token',
      e2eeKey: 'shared-e2ee-key'
    });
    const previewFrame = vi.fn();
    session.preview!.subscribe(previewFrame);
    hostChannel.port1.postMessage({
      kind: 'preview-frame',
      timestampUs: 123_456,
      keyFrame: true,
      data: Uint8Array.from([0, 0, 0, 1, 0x65])
    });
    expect(session.preview).toMatchObject({ width: 1920, height: 1080, frameRate: 60 });
    await vi.waitFor(() =>
      expect(previewFrame).toHaveBeenCalledWith(
        expect.objectContaining({ timestampUs: 123_456, keyFrame: true })
      )
    );

    await session.stop();

    expect(stopped).toHaveBeenCalledOnce();
  });
});
