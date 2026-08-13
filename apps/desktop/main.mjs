import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdtemp } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  app,
  BrowserWindow,
  desktopCapturer,
  dialog,
  ipcMain,
  MessageChannelMain,
  net,
  protocol,
  session,
  shell,
} from "electron";
import {
  APP_ORIGIN,
  createFrontendProtocolHandler,
} from "./frontend_protocol.mjs";
import {
  gameCaptureCancelListSourcesChannel,
  gameCaptureListSourcesChannel,
  gameCapturePublisherChannel,
  gameCaptureStartChannel,
  MacOSGameCaptureSourceOffers,
  parseGameCapturePublisherRequest,
  parseGameCapturePublisherStatus,
  parseMacOSGameCaptureSources,
  supportsMacOSGameCapture,
} from "./game_capture.mjs";
import { hasAppOrigin, isDesktopPermissionAllowed } from "./security.mjs";

const desktopRoot = path.dirname(fileURLToPath(import.meta.url));
const frontendRoot = app.isPackaged
  ? path.join(process.resourcesPath, "build")
  : path.resolve(desktopRoot, "../frontend/build");

let mainWindow;
let activeGameCaptureSession;
let activeGameCaptureSourceList;
const gameCaptureSourceOffers = new MacOSGameCaptureSourceOffers();
const macOSCaptureProbeListFlag = "--chatto-macos-capture-probe-list";
const macOSCapturePocFlag = "--chatto-macos-capture-poc";

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
  void start().catch((error) => {
    console.error(error);
    app.exit(1);
  });
}

async function start() {
  await app.whenReady();

  if (process.argv.includes(macOSCapturePocFlag)) {
    try {
      await runMacOSCapturePoc();
    } catch (error) {
      console.error(error);
      await dialog.showMessageBox({
        type: "error",
        title: "Capture failed",
        message: "Chatto could not complete the capture proof of concept.",
        detail: error instanceof Error ? error.message : String(error),
      });
      process.exitCode = 1;
    }
    app.quit();
    return;
  }

  if (process.argv.includes(macOSCaptureProbeListFlag)) {
    await runMacOSCaptureProbeList();
    app.quit();
    return;
  }

  await protocol.handle(
    "chatto",
    createFrontendProtocolHandler(frontendRoot, (input) => net.fetch(input)),
  );
  configureSession(session.defaultSession);
  configureGameCaptureIPC();
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
  app.on("before-quit", () => {
    cancelActiveGameCaptureSourceList();
    stopActiveGameCaptureSession();
  });
}

async function runMacOSCapturePoc() {
  if (process.platform !== "darwin" || !app.isPackaged) {
    throw new Error(
      "The macOS capture proof of concept requires a packaged macOS app.",
    );
  }

  const sourceListOutput = await runMacOSCaptureHelper(["list-json"]);
  const sourceList = JSON.parse(sourceListOutput);
  if (sourceList.protocolVersion !== 1 || !Array.isArray(sourceList.windows)) {
    throw new Error(
      "The capture helper returned an unsupported source-list response.",
    );
  }
  if (sourceList.windows.length === 0) {
    await dialog.showMessageBox({
      type: "info",
      title: "No windows available",
      message: "Chatto did not find a visible application window to capture.",
      detail: "Open a window of at least 320×180 points and try again.",
    });
    return;
  }

  const source = await chooseCaptureWindow(sourceList.windows);
  if (!source) return;

  const captureDirectory = await mkdtemp(
    path.join(app.getPath("temp"), "chatto-capture-poc-"),
  );
  const output = path.join(captureDirectory, "capture.mov");
  const statusWindow = createCaptureStatusWindow(source);
  try {
    await runMacOSCaptureHelper([
      "capture",
      "--window",
      String(source.windowID),
      "--duration",
      "15",
      "--output",
      output,
    ]);
  } finally {
    if (!statusWindow.isDestroyed()) statusWindow.destroy();
  }

  const result = await dialog.showMessageBox({
    type: "info",
    title: "Capture complete",
    message: `Captured ${captureSourceLabel(source)} for 15 seconds.`,
    detail: `The temporary recording is at:\n${output}`,
    buttons: ["Open recording", "Reveal in Finder", "Done"],
    defaultId: 0,
    cancelId: 2,
    noLink: true,
  });
  if (result.response === 0) {
    const openError = await shell.openPath(output);
    if (openError) throw new Error(openError);
  } else if (result.response === 1) {
    shell.showItemInFolder(output);
  }
}

async function chooseCaptureWindow(windows) {
  const cancelId = windows.length;
  const result = await dialog.showMessageBox({
    type: "question",
    title: "Share a window",
    message: "Choose a window to capture",
    detail:
      "The proof of concept records its video and application audio for 15 seconds.",
    buttons: [...windows.map(captureSourceLabel), "Cancel"],
    defaultId: 0,
    cancelId,
    noLink: true,
  });
  return result.response === cancelId ? undefined : windows[result.response];
}

function createCaptureStatusWindow(source) {
  const status = new BrowserWindow({
    title: "Capturing window",
    width: 440,
    height: 220,
    resizable: false,
    closable: false,
    minimizable: false,
    backgroundColor: "#111827",
    show: false,
    webPreferences: secureWebPreferences(),
  });
  status.removeMenu();
  status.once("ready-to-show", () => status.show());
  void status.loadURL(
    `data:text/html;charset=utf-8,${encodeURIComponent(captureStatusHTML(source))}`,
  );
  return status;
}

function captureStatusHTML(source) {
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
  <style>
    :root { color-scheme: dark; font-family: -apple-system, BlinkMacSystemFont, sans-serif; }
    body { margin: 0; min-height: 100vh; display: grid; place-content: center; padding: 24px;
      box-sizing: border-box; background: #111827; color: #f9fafb; text-align: center; }
    h1 { margin: 0 0 10px; font-size: 22px; }
    p { margin: 0; color: #d1d5db; }
    small { display: block; margin-top: 14px; color: #9ca3af; }
  </style>
</head>
<body><main><h1>Capturing ${escapeHTML(source.applicationName)}</h1>
<p>${escapeHTML(source.title || "Untitled window")}</p><small>Recording for 15 seconds…</small></main></body>
</html>`;
}

function captureSourceLabel(source) {
  return source.title
    ? `${source.applicationName} — ${source.title}`
    : source.applicationName;
}

function escapeHTML(value) {
  return String(value).replace(
    /[&<>"']/g,
    (character) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        character
      ],
  );
}

function runMacOSCaptureHelper(arguments_) {
  const executable = macOSCaptureHelperExecutable();
  return new Promise((resolve, reject) => {
    const child = spawn(executable, arguments_, {
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
      if (stdout.length > 2 * 1024 * 1024) child.kill();
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
      if (stderr.length > 2 * 1024 * 1024) child.kill();
    });
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) {
        resolve(stdout);
        return;
      }
      reject(
        new Error(
          stderr.trim() ||
            (signal
              ? `The macOS capture helper exited after signal ${signal}.`
              : `The macOS capture helper exited with status ${code}.`),
        ),
      );
    });
  });
}

function runMacOSCaptureHelperBinary(arguments_) {
  cancelActiveGameCaptureSourceList();
  const executable = macOSCaptureHelperExecutable();
  return new Promise((resolve, reject) => {
    const child = spawn(executable, arguments_, {
      stdio: ["ignore", "pipe", "pipe"],
    });
    const chunks = [];
    let stdoutLength = 0;
    let stderr = "";
    let stdoutTooLarge = false;
    let stderrTooLarge = false;
    let settled = false;
    let timedOut = false;
    let forceStopTimer;
    const sourceList = { child, cancel };
    activeGameCaptureSourceList = sourceList;
    const timeout = setTimeout(() => {
      timedOut = true;
      cancel();
    }, 30_000);
    timeout.unref();
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      if (stdoutTooLarge) return;
      if (stdoutLength + chunk.length > 16 * 1024 * 1024) {
        stdoutTooLarge = true;
        cancel();
        return;
      }
      stdoutLength += chunk.length;
      chunks.push(chunk);
    });
    child.stderr.on("data", (chunk) => {
      if (stderrTooLarge) return;
      if (stderr.length + chunk.length > 2 * 1024 * 1024) {
        stderrTooLarge = true;
        cancel();
        return;
      }
      stderr += chunk;
    });
    child.once("error", (error) => finish(() => reject(error)));
    child.once("exit", (code, signal) => {
      finish(() => {
        if (timedOut) {
          reject(
            new Error(
              "The macOS capture helper timed out while listing sources.",
            ),
          );
          return;
        }
        if (stdoutTooLarge) {
          reject(new Error("The macOS capture helper returned too much data."));
          return;
        }
        if (stderrTooLarge) {
          reject(
            new Error(
              "The macOS capture helper produced too much diagnostic output.",
            ),
          );
          return;
        }
        if (code === 0) {
          resolve(Buffer.concat(chunks, stdoutLength));
          return;
        }
        reject(
          new Error(
            stderr.trim() ||
              (signal
                ? `The macOS capture helper exited after signal ${signal}.`
                : `The macOS capture helper exited with status ${code}.`),
          ),
        );
      });
    });

    function cancel() {
      if (child.exitCode === null && child.signalCode === null) {
        child.kill("SIGTERM");
        forceStopTimer ??= setTimeout(() => {
          if (child.exitCode === null && child.signalCode === null)
            child.kill("SIGKILL");
        }, 1_000);
        forceStopTimer.unref();
      }
    }

    function finish(callback) {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      clearTimeout(forceStopTimer);
      if (activeGameCaptureSourceList === sourceList)
        activeGameCaptureSourceList = undefined;
      callback();
    }
  });
}

function cancelActiveGameCaptureSourceList() {
  activeGameCaptureSourceList?.cancel();
  activeGameCaptureSourceList = undefined;
  gameCaptureSourceOffers.clear();
}

function macOSCaptureHelperExecutable() {
  return path.resolve(
    process.resourcesPath,
    "../Helpers/Chatto Capture Helper.app/Contents/MacOS/chatto-macos-capture-probe",
  );
}

async function runMacOSCaptureProbeList() {
  if (process.platform !== "darwin" || !app.isPackaged) {
    throw new Error("The macOS capture probe requires a packaged macOS app.");
  }

  const executable = macOSCaptureHelperExecutable();
  await new Promise((resolve, reject) => {
    const child = spawn(executable, ["list"], { stdio: "inherit" });
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(
        new Error(
          signal
            ? `The macOS capture probe exited after signal ${signal}.`
            : `The macOS capture probe exited with status ${code}.`,
        ),
      );
    });
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
    webPreferences: mainWindowWebPreferences(),
  });

  protectNavigation(window);
  window.on("closed", () => {
    if (mainWindow === window) {
      cancelActiveGameCaptureSourceList();
      stopActiveGameCaptureSession();
      mainWindow = undefined;
    }
  });
  window.loadURL(APP_ORIGIN);
  return window;
}

function mainWindowWebPreferences() {
  const preferences = secureWebPreferences();
  if (isGameCaptureAvailable()) {
    preferences.preload = path.join(desktopRoot, "preload.cjs");
  }
  return preferences;
}

function configureGameCaptureIPC() {
  if (!isGameCaptureAvailable()) return;

  ipcMain.handle(gameCaptureListSourcesChannel, async (event) => {
    if (
      !mainWindow ||
      event.sender !== mainWindow.webContents ||
      !hasAppOrigin(event.senderFrame.url)
    ) {
      throw new Error(
        "Native screen-share sources are available only to the Chatto renderer.",
      );
    }

    const output = await runMacOSCaptureHelperBinary(["list-previews"]);
    return gameCaptureSourceOffers.replace(
      parseMacOSGameCaptureSources(output),
    );
  });

  ipcMain.on(gameCaptureCancelListSourcesChannel, (event) => {
    if (
      !mainWindow ||
      event.sender !== mainWindow.webContents ||
      !hasAppOrigin(event.senderFrame.url)
    ) {
      return;
    }
    cancelActiveGameCaptureSourceList();
  });

  ipcMain.on(gameCaptureStartChannel, (event, request) => {
    if (
      !mainWindow ||
      event.sender !== mainWindow.webContents ||
      !hasAppOrigin(event.senderFrame.url)
    ) {
      return;
    }

    const requestId =
      request && typeof request.requestId === "string" ? request.requestId : "";
    const sourceId =
      request && typeof request.sourceId === "string" ? request.sourceId : "";
    if (!requestId || requestId.length > 128) return;

    const { port1, port2 } = new MessageChannelMain();
    event.senderFrame.postMessage(gameCapturePublisherChannel, { requestId }, [
      port2,
    ]);

    try {
      const publisherRequest = parseGameCapturePublisherRequest(request);
      const source = gameCaptureSourceOffers.consume(publisherRequest.sourceId);
      startGameCaptureSession(source, publisherRequest, port1);
    } catch (error) {
      port1.postMessage({
        kind: "error",
        message:
          error instanceof Error
            ? error.message
            : "Native screen sharing could not start.",
      });
      port1.close();
    }
  });
}

function startGameCaptureSession(source, publisherRequest, port) {
  stopActiveGameCaptureSession();
  const sourceArguments = [
    source.kind === "window" ? "--window" : "--display",
    String(source.nativeId),
  ];
  if (source.kind === "window") {
    sourceArguments.push(
      "--expected-window-bundle",
      source.expectedBundleIdentifier,
    );
  }
  const child = spawn(
    macOSCaptureHelperExecutable(),
    [
      "publish",
      ...sourceArguments,
      "--fps",
      "60",
      "--max-width",
      "1920",
      "--max-height",
      "1080",
    ],
    { stdio: ["pipe", "pipe", "pipe"] },
  );
  let stdout = "";
  let stderr = "";
  let stopping = false;
  let forceStopTimer;
  const session = { child, port, stop };
  activeGameCaptureSession = session;

  child.stdout.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    stdout += chunk;
    if (stdout.length > 64 * 1024) {
      port.postMessage({
        kind: "error",
        message: "The native publisher returned too much status data.",
      });
      stop();
      return;
    }
    try {
      for (;;) {
        const lineEnd = stdout.indexOf("\n");
        if (lineEnd < 0) break;
        const line = stdout.slice(0, lineEnd).trim();
        stdout = stdout.slice(lineEnd + 1);
        if (line) port.postMessage(parseGameCapturePublisherStatus(line));
      }
    } catch {
      port.postMessage({
        kind: "error",
        message: "The native screen-share publisher returned invalid status.",
      });
      stop();
    }
  });
  child.stderr.setEncoding("utf8");
  child.stderr.on("data", (chunk) => {
    stderr += chunk;
    if (stderr.length > 64 * 1024) stderr = stderr.slice(-64 * 1024);
  });
  child.once("error", () => {
    port.postMessage({
      kind: "error",
      message: "The native screen-share helper could not start.",
    });
  });
  child.once("exit", (code, signal) => {
    clearTimeout(forceStopTimer);
    if (activeGameCaptureSession === session)
      activeGameCaptureSession = undefined;
    if (!stopping && (code !== 0 || signal)) {
      port.postMessage({
        kind: "error",
        message:
          stderr.trim() ||
          "The native screen-share helper stopped unexpectedly.",
      });
    }
    port.postMessage({ kind: "ended" });
    port.close();
  });
  port.on("message", (event) => {
    if (event.data?.kind === "stop") stop();
  });
  port.on("close", stop);
  port.start();

  child.stdin.end(
    JSON.stringify({
      protocolVersion: 1,
      livekitURL: publisherRequest.livekitUrl,
      token: publisherRequest.token,
      e2eeKey: publisherRequest.e2eeKey,
    }),
  );

  function stop() {
    if (stopping) return;
    stopping = true;
    if (activeGameCaptureSession === session)
      activeGameCaptureSession = undefined;
    if (child.exitCode === null && child.signalCode === null) {
      child.kill("SIGTERM");
      forceStopTimer = setTimeout(() => {
        if (child.exitCode === null && child.signalCode === null)
          child.kill("SIGKILL");
      }, 3_000);
      forceStopTimer.unref();
    }
  }
}

function stopActiveGameCaptureSession() {
  activeGameCaptureSession?.stop();
  activeGameCaptureSession = undefined;
}

function isGameCaptureAvailable() {
  return (
    process.platform === "darwin" &&
    app.isPackaged &&
    supportsMacOSGameCapture(process.getSystemVersion()) &&
    existsSync(macOSCaptureHelperExecutable())
  );
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
