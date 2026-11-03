/** Native image and file-link actions. The listener lives with the main window. */
import { hasAppOrigin } from './security.mjs';

/** Allow web assets and local image snapshots, without invoking protocol handlers. */
function downloadable(url) {
  try {
    return ['https:', 'http:', 'blob:', 'data:'].includes(new URL(url).protocol);
  } catch {
    return false;
  }
}

/** Use Chromium's image clipboard and download commands for the actual hit target. */
export function configureMediaContextMenu(window, Menu) {
  const contents = window.webContents;
  contents.on('context-menu', (_event, params) => {
    if (!hasAppOrigin(contents.getURL())) return;
    const items = [];
    if (params.mediaType === 'image') {
      items.push({
        label: 'Copy image',
        enabled: params.hasImageContents,
        click: () => contents.copyImageAt(params.x, params.y)
      });
      if (downloadable(params.srcURL)) {
        items.push({
          label: 'Save image as…',
          click: () => contents.downloadURL(params.srcURL)
        });
      }
    }
    if (downloadable(params.linkURL)) {
      items.push({
        label: 'Save link as…',
        click: () => contents.downloadURL(params.linkURL)
      });
    }
    if (items.length) Menu.buildFromTemplate(items).popup({ window, frame: params.frame });
  });
}
