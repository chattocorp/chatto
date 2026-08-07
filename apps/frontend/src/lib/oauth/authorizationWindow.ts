export interface AuthorizationWindow {
  readonly messageSource: Window | null;
  close(): Promise<void>;
  isClosed(): Promise<boolean>;
  navigate(url: string): Promise<void>;
  detachOpener(): void;
}

interface DesktopBindings {
  chattoOpenOAuthWindow(): Promise<number>;
  chattoNavigateOAuthWindow(windowId: number, url: string): Promise<void>;
  chattoIsOAuthWindowClosed(windowId: number): Promise<boolean>;
  chattoCloseOAuthWindow(windowId: number): Promise<void>;
}

type DesktopGlobal = typeof globalThis & { bindings?: DesktopBindings };

/** Whether the official frontend is hosted by Chatto's Deno Desktop shell. */
export function hasDesktopOAuthWindowBridge(): boolean {
  return Boolean((globalThis as DesktopGlobal).bindings);
}

/** Open a native CEF window through the desktop host's per-window bindings. */
export async function openDesktopAuthorizationWindow(): Promise<AuthorizationWindow> {
  const bindings = (globalThis as DesktopGlobal).bindings;
  if (!bindings) throw new Error('The desktop sign-in window bridge is unavailable.');

  const windowId = await bindings.chattoOpenOAuthWindow();
  return {
    messageSource: null,
    close: () => bindings.chattoCloseOAuthWindow(windowId),
    isClosed: () => bindings.chattoIsOAuthWindowClosed(windowId),
    navigate: (url) => bindings.chattoNavigateOAuthWindow(windowId, url),
    detachOpener: () => {}
  };
}

/** Wrap a browser popup in the same lifecycle used by the desktop bridge. */
export function browserAuthorizationWindow(popup: Window): AuthorizationWindow {
  return {
    messageSource: popup,
    close: async () => {
      if (!popup.closed) popup.close();
    },
    isClosed: async () => popup.closed,
    navigate: async (url) => {
      popup.location.href = url;
    },
    detachOpener: () => {
      popup.opener = null;
    }
  };
}
