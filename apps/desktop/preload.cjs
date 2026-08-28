// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

const { contextBridge, ipcRenderer } = require("electron");

const gameCaptureListSourcesChannel = "chatto:game-capture:list-sources";
const gameCaptureCancelListSourcesChannel =
  "chatto:game-capture:cancel-list-sources";
const gameCaptureStartChannel = "chatto:game-capture:start";
const gameCapturePublisherChannel = "chatto:game-capture:publisher";

ipcRenderer.on(gameCapturePublisherChannel, (event, message) => {
  if (typeof message?.requestId !== "string" || event.ports.length !== 1)
    return;
  window.postMessage(
    { type: gameCapturePublisherChannel, requestId: message.requestId },
    window.origin,
    event.ports,
  );
});

contextBridge.exposeInMainWorld(
  "chattoDesktop",
  Object.freeze({
    screenShare: Object.freeze({
      listSources: () => {
        if (!navigator.userActivation?.isActive) {
          return Promise.reject(
            new Error("Native screen sharing requires a user gesture."),
          );
        }
        return ipcRenderer.invoke(gameCaptureListSourcesChannel);
      },
      cancelSourceList: () =>
        ipcRenderer.send(gameCaptureCancelListSourcesChannel),
      startPublisher: (request) => {
        const requestId = crypto.randomUUID();
        ipcRenderer.send(gameCaptureStartChannel, {
          sourceId: request?.sourceId,
          livekitUrl: request?.livekitUrl,
          token: request?.token,
          e2eeKey: request?.e2eeKey,
          requestId,
        });
        return requestId;
      },
    }),
  }),
);
