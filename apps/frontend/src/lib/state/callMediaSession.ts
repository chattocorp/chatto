/** The most recently joined call owns the document's system media controls. */
let activeSession: { owner: CallMediaSession } | undefined;

type CallMediaAction = 'togglemicrophone' | 'togglecamera' | 'hangup';

/** DOM types omit the conferencing actions; runtime support is checked per action. */
type CallBrowserMediaSession = MediaSession & {
  setActionHandler(action: CallMediaAction, handler: (() => void) | null): void;
};

/**
 * Expose a connected call to browser and OS media controls. Unsupported actions
 * and rejected state reports must not interrupt the call. This does not grant
 * background execution permission.
 */
export class CallMediaSession {
  private session: CallBrowserMediaSession | undefined;
  private actions: CallMediaAction[] = [];
  private microphoneActive: boolean | undefined;
  private cameraActive: boolean | undefined;

  constructor(
    private readonly handlers: {
      toggleMicrophone: () => void;
      toggleCamera: () => void;
      hangup: () => void;
    }
  ) {
    if (typeof navigator === 'undefined' || !navigator.mediaSession) return;
    activeSession?.owner.dispose();
    this.session = navigator.mediaSession as CallBrowserMediaSession;
    activeSession = { owner: this };
    this.register('hangup', handlers.hangup);
    this.report(() => {
      // Brand-only metadata avoids exposing private call details on the lock screen.
      if (typeof MediaMetadata !== 'undefined') {
        this.session!.metadata = new MediaMetadata({ title: 'Chatto' });
      }
      this.session!.playbackState = 'playing';
    });
  }

  /** Expose capture controls only while the current member can use that media. */
  setPermissions(canUseVoice: boolean, canUseCamera: boolean): void {
    if (activeSession?.owner !== this) return;
    this.setActionEnabled('togglemicrophone', canUseVoice, this.handlers.toggleMicrophone);
    this.setActionEnabled('togglecamera', canUseCamera, this.handlers.toggleCamera);
  }

  /** Report confirmed local capture state after LiveKit updates its publications. */
  update(microphoneActive: boolean, cameraActive: boolean): void {
    if (activeSession?.owner !== this) return;
    if (this.microphoneActive !== microphoneActive) {
      this.microphoneActive = microphoneActive;
      this.report(() => this.session?.setMicrophoneActive?.(microphoneActive));
    }
    if (this.cameraActive !== cameraActive) {
      this.cameraActive = cameraActive;
      this.report(() => this.session?.setCameraActive?.(cameraActive));
    }
  }

  /** Release only this call's controls; late cleanup cannot erase a newer call. */
  dispose(): void {
    if (activeSession?.owner !== this) return;
    this.update(false, false);
    for (const action of this.actions) {
      this.report(() => this.session?.setActionHandler(action, null));
    }
    this.report(() => {
      this.session!.metadata = null;
      this.session!.playbackState = 'none';
    });
    activeSession = undefined;
    this.actions = [];
  }

  private setActionEnabled(action: CallMediaAction, enabled: boolean, handler: () => void): void {
    if (enabled) {
      if (!this.actions.includes(action)) this.register(action, handler);
    } else if (this.actions.includes(action)) {
      this.actions = this.actions.filter((registered) => registered !== action);
      this.report(() => this.session?.setActionHandler(action, null));
    }
  }

  private register(action: CallMediaAction, handler: () => void): void {
    try {
      this.session!.setActionHandler(action, () => {
        if (activeSession?.owner === this && this.actions.includes(action)) handler();
      });
      this.actions.push(action);
    } catch {
      // Browsers can expose MediaSession without supporting every call action.
    }
  }

  /** Older browsers can throw synchronously; newer capture-state APIs return promises. */
  private report(operation: () => void | Promise<void>): void {
    try {
      void Promise.resolve(operation()).catch(() => {});
    } catch {
      // System media integration is optional; the in-app controls remain available.
    }
  }
}
