import { describe, expect, it } from 'vitest';
import { configureBundledHLSProvider } from './hls';

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
