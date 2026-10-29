/**
 * Block reload accelerators before Electron dispatches them to the renderer or
 * application menu. Development windows retain their normal reload shortcuts.
 * The listener belongs to the webContents and dies with the window.
 */
export function protectReloadShortcuts(webContents, production) {
  if (!production) return;
  webContents.on('before-input-event', (event, input) => {
    if (
      input.type === 'keyDown' &&
      !input.alt &&
      (input.key === 'F5' || ((input.meta || input.control) && input.key.toLowerCase() === 'r'))
    ) {
      event.preventDefault();
    }
  });
}
