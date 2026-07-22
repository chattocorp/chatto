import { describe, expect, it, vi } from 'vitest';
import { configureBundledHLSProvider, recoverFatalHLS, shouldAbortHLSRecovery } from './hls';

describe('configureBundledHLSProvider', () => {
  it('installs a bundled hls.js loader on HLS providers', async () => {
    const provider: { type: string; library?: () => Promise<unknown> } = { type: 'hls' };

    configureBundledHLSProvider(provider);

    expect(provider.library).toBeTypeOf('function');
    await expect(provider.library!()).resolves.toBeTruthy();
  });

  it('leaves other providers untouched', () => {
    const provider: { type: string; library?: unknown } = { type: 'video' };

    configureBundledHLSProvider(provider);

    expect(provider.library).toBeUndefined();
  });
});

describe('recoverFatalHLS', () => {
  it('destroys the failed session and falls back when refresh cannot recover it', async () => {
    const destroy = vi.fn();
    const retry = vi.fn();
    const fallback = vi.fn();

    await recoverFatalHLS({
      instance: { destroy },
      rejectedUrl: 'https://chat.example.test/old.m3u8',
      refreshUrl: async () => null,
      retry,
      fallback
    });

    expect(destroy).toHaveBeenCalledOnce();
    expect(retry).not.toHaveBeenCalled();
    expect(fallback).toHaveBeenCalledOnce();
  });

  it('allows one retry when refresh returns a different URL', async () => {
    const retry = vi.fn();
    const fallback = vi.fn();

    await recoverFatalHLS({
      rejectedUrl: 'https://chat.example.test/old.m3u8',
      refreshUrl: async () => 'https://chat.example.test/fresh.m3u8',
      retry,
      fallback
    });

    expect(retry).toHaveBeenCalledWith('https://chat.example.test/fresh.m3u8');
    expect(fallback).not.toHaveBeenCalled();
  });
});

describe('shouldAbortHLSRecovery', () => {
  it('stops non-fatal audio SourceBuffer append loops', () => {
    expect(
      shouldAbortHLSRecovery({
        fatal: false,
        type: 'mediaError',
        details: 'bufferAppendError'
      })
    ).toBe(true);
  });

  it('leaves ordinary recoverable errors to hls.js', () => {
    expect(
      shouldAbortHLSRecovery({ fatal: false, type: 'networkError', details: 'fragLoadError' })
    ).toBe(false);
  });
});
