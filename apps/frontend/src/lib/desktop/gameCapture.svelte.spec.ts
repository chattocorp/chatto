import { afterEach, describe, expect, it } from 'vitest';
import { isGameCaptureAvailable, listGameCaptureSources } from './gameCapture';

afterEach(() => {
  delete window.chattoDesktop;
});

describe('desktop game capture', () => {
  it('is available only when the desktop host exposes it', () => {
    expect(isGameCaptureAvailable()).toBe(false);

    window.chattoDesktop = {
      gameCapture: {
        listSources: async () => ({ protocolVersion: 1, sources: [] }),
        startPublisher: () => 'request-1'
      }
    };

    expect(isGameCaptureAvailable()).toBe(true);
  });

  it('returns validated opaque capture sources', async () => {
    const sources = [
      {
        id: 'window:42',
        kind: 'window' as const,
        applicationName: 'Example Game',
        bundleIdentifier: 'example.game',
        title: 'Main Menu',
        width: 1920,
        height: 1080
      }
    ];
    window.chattoDesktop = {
      gameCapture: {
        listSources: async () => ({ protocolVersion: 1, sources }),
        startPublisher: () => 'request-1'
      }
    };

    await expect(listGameCaptureSources()).resolves.toEqual(sources);
  });

  it('rejects malformed desktop responses', async () => {
    window.chattoDesktop = {
      gameCapture: {
        startPublisher: () => 'request-1',
        listSources: async () =>
          ({
            protocolVersion: 1,
            sources: [{ id: '', kind: 'window' }]
          }) as never
      }
    };

    await expect(listGameCaptureSources()).rejects.toThrow('invalid capture source');
  });
});
