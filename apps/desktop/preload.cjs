// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

const { contextBridge, ipcRenderer } = require('electron');

// These listeners belong to this document. Cancel both halves of a side-button
// click so Chromium cannot also navigate on release.
function handleMouseNavigation(event) {
  if (event.button !== 3 && event.button !== 4) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  if (event.type !== 'mousedown') return;
  if (event.button === 3) window.history.back();
  else window.history.forward();
}

window.addEventListener('mousedown', handleMouseNavigation, true);
window.addEventListener('mouseup', handleMouseNavigation, true);

// Mouse drivers can send navigation shortcuts instead of raw button events.
window.addEventListener(
  'keydown',
  (event) => {
    if (event.shiftKey || event.ctrlKey) return;
    const macShortcut = process.platform === 'darwin' && event.metaKey && !event.altKey;
    const altShortcut = event.altKey && !event.metaKey;
    const back = (macShortcut && event.key === '[') || (altShortcut && event.key === 'ArrowLeft');
    const forward =
      (macShortcut && event.key === ']') || (altShortcut && event.key === 'ArrowRight');
    if (!back && !forward) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (back) window.history.back();
    else window.history.forward();
  },
  true
);

const gameCaptureListSourcesChannel = 'chatto:game-capture:list-sources';
const gameCaptureCancelListSourcesChannel = 'chatto:game-capture:cancel-list-sources';
const gameCaptureStartChannel = 'chatto:game-capture:start';
const gameCapturePublisherChannel = 'chatto:game-capture:publisher';

ipcRenderer.on(gameCapturePublisherChannel, (event, message) => {
  if (typeof message?.requestId !== 'string' || event.ports.length !== 1) return;
  window.postMessage(
    { type: gameCapturePublisherChannel, requestId: message.requestId },
    window.origin,
    event.ports
  );
});

contextBridge.exposeInMainWorld(
  'chattoDesktop',
  Object.freeze({
    passwordLogin: (request) =>
      ipcRenderer.invoke('chatto:password-login', {
        serverUrl: request?.serverUrl,
        identifier: request?.identifier,
        password: request?.password
      }),
    screenShare: process.argv.includes('--chatto-native-screen-share')
      ? Object.freeze({
          listSources: () => {
            if (!navigator.userActivation?.isActive) {
              return Promise.reject(new Error('Native screen sharing requires a user gesture.'));
            }
            return ipcRenderer.invoke(gameCaptureListSourcesChannel);
          },
          cancelSourceList: () => ipcRenderer.send(gameCaptureCancelListSourcesChannel),
          startPublisher: (request) => {
            const requestId = crypto.randomUUID();
            ipcRenderer.send(gameCaptureStartChannel, {
              sourceId: request?.sourceId,
              livekitUrl: request?.livekitUrl,
              token: request?.token,
              e2eeKey: request?.e2eeKey,
              requestId
            });
            return requestId;
          }
        })
      : undefined
  })
);
