import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  app,
  BrowserWindow,
  desktopCapturer,
  dialog,
  net,
  protocol,
  session,
  shell,
} from "electron";
import {
  APP_ORIGIN,
  createFrontendProtocolHandler,
} from "./frontend_protocol.mjs";
import { hasAppOrigin, isDesktopPermissionAllowed } from "./security.mjs";

const desktopRoot = path.dirname(fileURLToPath(import.meta.url));
const frontendRoot = app.isPackaged
  ? path.join(process.resourcesPath, "build")
  : path.resolve(desktopRoot, "../frontend/build");

let mainWindow;

protocol.registerSchemesAsPrivileged([
  {
    scheme: "chatto",
    privileges: {
      standard: true,
      secure: true,
      allowServiceWorkers: true,
      supportFetchAPI: true,
      corsEnabled: true,
      stream: true,
      codeCache: true,
    },
  },
]);

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  // Electron does not finish becoming ready until the ESM main module has
  // loaded, so awaiting app.whenReady() at module scope would deadlock startup.
  void start();
}

async function start() {
  await app.whenReady();

  await protocol.handle(
    "chatto",
    createFrontendProtocolHandler(frontendRoot, (input) => net.fetch(input)),
  );
  configureSession(session.defaultSession);
  mainWindow = createMainWindow();

  app.on("second-instance", () => {
    if (!mainWindow) return;
    if (mainWindow.isMinimized()) mainWindow.restore();
    mainWindow.focus();
  });
  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0)
      mainWindow = createMainWindow();
  });
  app.on("window-all-closed", () => {
    if (process.platform !== "darwin") app.quit();
  });
}

function createMainWindow() {
  const window = new BrowserWindow({
    title: "Chatto Desktop",
    width: 1280,
    height: 820,
    minWidth: 800,
    minHeight: 600,
    backgroundColor: "#111827",
    icon: path.join(frontendRoot, "icons/icon-512.png"),
    webPreferences: secureWebPreferences(),
  });

  protectNavigation(window);
  window.on("closed", () => {
    if (mainWindow === window) mainWindow = undefined;
  });
  window.loadURL(APP_ORIGIN);
  return window;
}

function secureWebPreferences() {
  return {
    contextIsolation: true,
    nodeIntegration: false,
    sandbox: true,
  };
}

function protectNavigation(window) {
  window.webContents.on("will-navigate", (event, target) => {
    if (hasAppOrigin(target)) return;
    event.preventDefault();
    if (isWebUrl(target)) void shell.openExternal(target);
  });

  window.webContents.setWindowOpenHandler(({ url }) => {
    if (url !== "about:blank") {
      if (isWebUrl(url)) void shell.openExternal(url);
      return { action: "deny" };
    }
    return {
      action: "allow",
      overrideBrowserWindowOptions: { webPreferences: secureWebPreferences() },
    };
  });

  window.webContents.on("did-create-window", (child) => {
    child.webContents.on("will-navigate", (event, target) => {
      if (isWebUrl(target) || hasAppOrigin(target)) return;
      event.preventDefault();
    });
    child.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  });
}

function configureSession(appSession) {
  appSession.setPermissionCheckHandler((_contents, permission, origin) =>
    isDesktopPermissionAllowed(permission, origin),
  );
  appSession.setPermissionRequestHandler((contents, permission, callback) => {
    callback(isDesktopPermissionAllowed(permission, contents.getURL()));
  });
  appSession.setDisplayMediaRequestHandler(async (request, callback) => {
    if (!hasAppOrigin(request.securityOrigin) || !request.userGesture) {
      callback({});
      return;
    }

    try {
      const sources = await desktopCapturer.getSources({
        types: ["screen", "window"],
      });
      if (sources.length === 0) {
        callback({});
        return;
      }
      const choice = await dialog.showMessageBox(mainWindow, {
        type: "question",
        title: "Share your screen",
        message: "Choose what Chatto may share",
        buttons: [...sources.map((source) => source.name), "Cancel"],
        cancelId: sources.length,
        defaultId: 0,
        noLink: true,
      });
      const source = sources[choice.response];
      callback(source ? { video: source } : {});
    } catch {
      callback({});
    }
  });
}

function isWebUrl(value) {
  try {
    return ["http:", "https:"].includes(new URL(value).protocol);
  } catch {
    return false;
  }
}
