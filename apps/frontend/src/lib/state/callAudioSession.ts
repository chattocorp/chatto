/** Audio Session is not yet included in the project's DOM types. */
interface BrowserAudioSession {
  type: 'auto' | 'playback' | 'transient' | 'transient-solo' | 'ambient' | 'play-and-record';
}

/** Release the previous call's audio intent before a replacement call takes ownership. */
let releaseActiveSession: (() => void) | undefined;

/**
 * Request call audio handling before capture and playback start. An explicit
 * type lets supported browsers retain Web Audio playback in the background,
 * including while the microphone is muted. It does not prevent page termination.
 * Return an idempotent cleanup that restores the previous type.
 */
export function beginCallAudioSession(): () => void {
  try {
    if (typeof navigator === 'undefined') return () => {};
    const session = (navigator as Navigator & { audioSession?: BrowserAudioSession }).audioSession;
    if (!session) return () => {};

    releaseActiveSession?.();
    const previousType = session.type;
    session.type = 'play-and-record';

    const release = () => {
      if (releaseActiveSession !== release) return;
      releaseActiveSession = undefined;
      try {
        // Do not overwrite a different audio intent set by another feature.
        if (session.type === 'play-and-record') session.type = previousType;
      } catch {
        // Optional browser audio handling must not block call cleanup.
      }
    };
    releaseActiveSession = release;
    return release;
  } catch {
    // Experimental API support can be incomplete even when the property exists.
    return () => {};
  }
}
