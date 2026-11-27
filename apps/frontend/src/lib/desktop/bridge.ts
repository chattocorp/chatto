/** Optional capabilities exposed by the desktop preload. Browsers provide none of them. */
import type {
  NativeScreenSharePublisherRequest,
  NativeScreenShareSourceResponse
} from './nativeScreenShare';
import type { DesktopUpdatesBridge } from './updates.svelte';

export type CallbackRegistrationFailure =
  'callback-registration' | 'missing-xdg-mime' | 'install-in-applications';

export interface ChattoDesktopBridge {
  /** Subscribe to native popup closures, identified by the original window name. */
  onAuthorizationWindowClosed?: (callback: (frameName: string) => void) => () => void;
  /** Open an authorization request in the system browser and wait for its deep link. */
  browserAuthorization?: {
    start(origin: string, url: string): Promise<string | { error: CallbackRegistrationFailure }>;
    cancel(state: string): void;
  };
  /** The token identifies one call and binding lifetime across queued IPC messages. */
  muteShortcut?: {
    available(chord: string): Promise<boolean>;
    set(chord: string, token: string): Promise<boolean>;
    onPress(callback: (token: string) => void): () => void;
  };
  updates?: DesktopUpdatesBridge;
  /** Native alerts carry opaque click IDs, never host-provided navigation URLs. */
  notifications?: {
    show(request: { id: string; title: string; body: string }): Promise<void>;
    clear(ids: string[]): void;
    onClick(callback: (id: string) => void): () => void;
  };
  /** Host-owned password request; the renderer receives only the resulting session. */
  passwordLogin?: (request: {
    serverUrl: string;
    identifier: string;
    password: string;
  }) => Promise<unknown>;
  screenShare?: {
    listSources(): Promise<NativeScreenShareSourceResponse>;
    cancelSourceList?(): void;
    startPublisher(request: NativeScreenSharePublisherRequest): string;
  };
}

declare global {
  interface Window {
    chattoDesktop?: ChattoDesktopBridge;
  }
}
