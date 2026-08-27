import type { HLSProvider } from 'vidstack';

/** Configure Vidstack to load the bundled hls.js package instead of a CDN. */
export function configureBundledHLSProvider(provider: unknown): void {
  if (!provider || typeof provider !== 'object' || !('type' in provider)) return;
  if ((provider as { type?: unknown }).type !== 'hls') return;
  (provider as HLSProvider).library = () => import('hls.js');
}

export function shouldAbortHLSRecovery(error: {
  fatal?: boolean;
  type?: string;
  details?: string;
}): boolean {
  return (
    error.fatal === true ||
    (error.type === 'mediaError' && error.details === 'bufferAppendError')
  );
}

/** Stop an unrecoverable HLS session before attempting one controlled retry. */
export async function recoverFatalHLS({
  instance,
  rejectedUrl,
  refreshUrl,
  retry,
  fallback
}: {
  instance?: { destroy?: () => void } | null;
  rejectedUrl: string;
  refreshUrl?: () => void | Promise<string | null>;
  retry: (url: string) => void;
  fallback: () => void;
}): Promise<void> {
  instance?.destroy?.();
  try {
    const refreshedUrl = await refreshUrl?.();
    if (typeof refreshedUrl === 'string' && refreshedUrl !== rejectedUrl) {
      retry(refreshedUrl);
      return;
    }
  } catch {
    // Fall through to the MP4 rendition when ticket refresh also fails.
  }
  fallback();
}
