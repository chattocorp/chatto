// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

const { contextBridge, ipcRenderer } = require("electron");

const gameCaptureListSourcesChannel = "chatto:game-capture:list-sources";
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
    gameCapture: Object.freeze({
      listSources: () => ipcRenderer.invoke(gameCaptureListSourcesChannel),
      startPublisher: (request) => {
        const requestId = crypto.randomUUID();
        ipcRenderer.send(gameCaptureStartChannel, { requestId, ...request });
        return requestId;
      },
    }),
  }),
);
