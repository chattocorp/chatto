import { createFrontendHandler } from "./frontend_server.ts";
import { oauthNavigationUrl } from "./oauth_window.ts";

const OAUTH_WINDOW_WIDTH = 520;
const OAUTH_WINDOW_HEIGHT = 680;

type OAuthWindows = Map<number, Deno.BrowserWindow>;

/** Adopt the startup window and expose the native OAuth-window bridge. */
export function createMainWindow(): Deno.BrowserWindow {
  const mainWindow = new Deno.BrowserWindow({ title: "Chatto Desktop" });
  bindOAuthWindows(mainWindow);
  return mainWindow;
}

function bindOAuthWindows(mainWindow: Deno.BrowserWindow): void {
  const windows: OAuthWindows = new Map();

  mainWindow.bind("chattoOpenOAuthWindow", () => {
    const oauthWindow = new Deno.BrowserWindow({
      title: "Chatto Desktop",
      width: OAUTH_WINDOW_WIDTH,
      height: OAUTH_WINDOW_HEIGHT,
    });
    const windowId = oauthWindow.windowId;
    windows.set(windowId, oauthWindow);
    oauthWindow.addEventListener("close", () => windows.delete(windowId));
    return Promise.resolve(windowId);
  });

  mainWindow.bind(
    "chattoNavigateOAuthWindow",
    (windowId: unknown, target: unknown) => {
      const oauthWindow = requireOAuthWindow(windows, windowId);
      oauthWindow.navigate(oauthNavigationUrl(target));
      return Promise.resolve();
    },
  );

  mainWindow.bind("chattoIsOAuthWindowClosed", (windowId: unknown) => {
    if (!Number.isInteger(windowId)) return Promise.resolve(true);
    const oauthWindow = windows.get(windowId as number);
    return Promise.resolve(!oauthWindow || oauthWindow.isClosed());
  });

  mainWindow.bind("chattoCloseOAuthWindow", (windowId: unknown) => {
    if (!Number.isInteger(windowId)) return Promise.resolve();
    const oauthWindow = windows.get(windowId as number);
    if (oauthWindow && !oauthWindow.isClosed()) oauthWindow.close();
    windows.delete(windowId as number);
    return Promise.resolve();
  });

  mainWindow.addEventListener("close", () => {
    for (const oauthWindow of windows.values()) {
      if (!oauthWindow.isClosed()) oauthWindow.close();
    }
    windows.clear();
  });
}

function requireOAuthWindow(
  windows: OAuthWindows,
  windowId: unknown,
): Deno.BrowserWindow {
  if (!Number.isInteger(windowId)) throw new TypeError("Invalid window ID.");
  const oauthWindow = windows.get(windowId as number);
  if (!oauthWindow || oauthWindow.isClosed()) {
    throw new Error("The sign-in window is no longer open.");
  }
  return oauthWindow;
}

if (import.meta.main) {
  createMainWindow();
  Deno.serve(createFrontendHandler());
}
