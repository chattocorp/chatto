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

/** Size an authorization popup for the form while keeping it within the screen. */
export function authorizationWindowFeatures(owner: Window): string {
  const width = Math.max(1, Math.min(560, owner.screen.availWidth - 32));
  const height = Math.max(1, Math.min(760, owner.screen.availHeight - 100));
  const left = Math.max(0, Math.round(owner.screenX + (owner.outerWidth - width) / 2));
  const top = Math.max(0, Math.round(owner.screenY + (owner.outerHeight - height) / 2));
  return `popup,width=${width},height=${height},left=${left},top=${top}`;
}
