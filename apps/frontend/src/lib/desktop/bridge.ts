/** Optional capabilities exposed by the desktop preload. Browsers provide none of them. */
import type {
  NativeScreenSharePublisherRequest,
  NativeScreenShareSourceResponse
} from './nativeScreenShare';
import type { DesktopUpdatesBridge } from './updates.svelte';

export interface ChattoDesktopBridge {
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
