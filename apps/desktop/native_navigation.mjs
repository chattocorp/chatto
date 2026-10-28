// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

/**
 * Handle macOS navigation gestures, including mouse drivers that send native
 * swipes instead of DOM button events. The listener lives with the window.
 * Native swipe directions use left for back and right for forward.
 */
export function configureNativeNavigation(window, platform = process.platform) {
  if (platform !== 'darwin') return;
  window.on('swipe', (_event, direction) => {
    const history = window.webContents.navigationHistory;
    if (direction === 'left' && history.canGoBack()) history.goBack();
    else if (direction === 'right' && history.canGoForward()) history.goForward();
  });
}
