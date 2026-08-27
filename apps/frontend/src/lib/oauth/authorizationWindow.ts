export interface AuthorizationWindow {
  readonly messageSource: Window | null;
  close(): Promise<void>;
  isClosed(): Promise<boolean>;
  navigate(url: string): Promise<void>;
  detachOpener(): void;
}

/** Wrap a browser popup in the lifecycle used by the OAuth flow. */
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
