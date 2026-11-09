/** Audio Session is not yet included in the project's DOM types. */
interface BrowserAudioSession {
  type: 'auto' | 'playback' | 'transient' | 'transient-solo' | 'ambient' | 'play-and-record';
}

/** All calls in this document share one browser audio session. */
let activeSession:
  | { session: BrowserAudioSession; previousType: BrowserAudioSession['type']; owners: number }
  | undefined;

/**
 * Request simultaneous capture and playback before a call starts its media.
 * This is a best-effort audio intent, not background execution permission.
 * No capture device is opened. The last call restores the previous mode unless
 * another feature changed it. The returned release function is idempotent.
 */
export function beginCallAudioSession(): () => void {
  try {
    if (typeof navigator === 'undefined') return () => {};
    const session = (navigator as Navigator & { audioSession?: BrowserAudioSession }).audioSession;
    if (!session) return () => {};

    if (activeSession?.session !== session) {
      const previousType = session.type;
      session.type = 'play-and-record';
      activeSession = { session, previousType, owners: 0 };
    }
    const ownedSession = activeSession;
    ownedSession.owners += 1;
    let released = false;

    return () => {
      if (released) return;
      released = true;
      ownedSession.owners -= 1;
      if (ownedSession.owners > 0) return;
      if (activeSession === ownedSession) activeSession = undefined;
      try {
        if (session.type === 'play-and-record') session.type = ownedSession.previousType;
      } catch {
        // Optional browser audio handling must not block call cleanup.
      }
    };
  } catch {
    // Missing or rejecting Audio Session APIs must not prevent calls.
    return () => {};
  }
}
