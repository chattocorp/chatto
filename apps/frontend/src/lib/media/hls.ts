import type { HLSProvider } from 'vidstack';

/** Configure Vidstack to load the bundled hls.js package instead of a CDN. */
export function configureBundledHLSProvider(provider: unknown): void {
  if (!provider || typeof provider !== 'object' || !('type' in provider)) return;
  if ((provider as { type?: unknown }).type !== 'hls') return;
  (provider as HLSProvider).library = () => import('hls.js');
}
