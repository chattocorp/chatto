// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import { readFileSync, writeFileSync } from 'node:fs';
import { APP_ORIGIN } from './frontend_protocol.mjs';
import { hasAppOrigin } from './security.mjs';

/** Keep only a server root. The frontend owns per-server room restoration. */
function serverPath(value) {
  if (!hasAppOrigin(value)) return null;
  const url = new URL(value);
  const segment = url.pathname.split('/')[2];
  if (!url.pathname.startsWith('/chat/') || !segment) return null;
  if (['servers', 'notifications', 'preferences'].includes(segment)) return null;
  if (!/^[a-zA-Z0-9.\[\]:_-]+$/.test(segment)) return null;
  return `/chat/${segment}`;
}

/**
 * Restore and remember the main window's server in its desktop profile.
 * Only completed main-frame navigation is observed. Storage is best effort;
 * synchronous, small writes preserve the latest selection even on immediate quit.
 * Queries, fragments, and child routes never enter the saved record.
 */
export function configureLastServer(webContents, file) {
  let savedPath;
  try {
    savedPath = serverPath(`${APP_ORIGIN}${JSON.parse(readFileSync(file, 'utf8'))}`);
  } catch {
    // A missing or damaged preference must not prevent application startup.
  }
  const initialURL = savedPath ? `${APP_ORIGIN}${savedPath}` : APP_ORIGIN;

  const remember = (url) => {
    const next = serverPath(url);
    if (!next || next === savedPath) return;
    try {
      writeFileSync(file, JSON.stringify(next), { mode: 0o600 });
      savedPath = next;
    } catch {
      // Navigation must still work when profile storage is unavailable.
    }
  };
  webContents.on('did-navigate', (_event, url) => remember(url));
  webContents.on('did-navigate-in-page', (_event, url, isMainFrame) => {
    if (isMainFrame) remember(url);
  });
  return initialURL;
}
