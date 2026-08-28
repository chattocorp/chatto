import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  cancelNativeScreenShareSourceList,
  isNativeScreenShareAvailable,
  listNativeScreenShareSources
} from './nativeScreenShare';

afterEach(() => {
  delete window.chattoDesktop;
});

describe('native desktop screen sharing', () => {
  it('is available only when the desktop host exposes it', () => {
    expect(isNativeScreenShareAvailable()).toBe(false);

    window.chattoDesktop = { screenShare: {} } as never;
    expect(isNativeScreenShareAvailable()).toBe(false);

    window.chattoDesktop = {
      screenShare: {
        listSources: async () => ({ protocolVersion: 1, sources: [] }),
        startPublisher: () => 'request-1'
      }
    };

    expect(isNativeScreenShareAvailable()).toBe(true);
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
        height: 1080,
        preview: new Uint8Array([1, 2, 3])
      },
      {
        id: 'display:7',
        kind: 'display' as const,
        displayIndex: 1,
        isMainDisplay: true,
        width: 3024,
        height: 1964,
        preview: new Uint8Array([4, 5, 6])
      }
    ];
    window.chattoDesktop = {
      screenShare: {
        listSources: async () => ({ protocolVersion: 1, sources }),
        startPublisher: () => 'request-1'
      }
    };

    await expect(listNativeScreenShareSources()).resolves.toEqual(sources);
  });

  it('rejects malformed desktop responses', async () => {
    window.chattoDesktop = {
      screenShare: {
        startPublisher: () => 'request-1',
        listSources: async () =>
          ({
            protocolVersion: 1,
            sources: [{ id: '', kind: 'window' }]
          }) as never
      }
    };

    await expect(listNativeScreenShareSources()).rejects.toThrow('invalid capture source');
  });

  it('asks the host to cancel a superseded enumeration when supported', () => {
    const cancelSourceList = vi.fn();
    window.chattoDesktop = {
      screenShare: {
        listSources: async () => ({ protocolVersion: 1, sources: [] }),
        cancelSourceList,
        startPublisher: () => 'request-1'
      }
    };

    cancelNativeScreenShareSourceList();

    expect(cancelSourceList).toHaveBeenCalledOnce();
  });
});
