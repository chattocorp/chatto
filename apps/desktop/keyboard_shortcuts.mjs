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

/** Hold at most one OS binding. Renderer loss and document navigation release it. */
export function configureMuteShortcut({ ipcMain, globalShortcut, contents, isTrusted, wayland }) {
  const validChord =
    /^(?=CommandOrControl\+|Control\+|Super\+|Alt\+)(?:CommandOrControl\+|(?:Control\+)?(?:Super\+)?)?(?:Alt\+)?(?:Shift\+)?(?:[A-Z0-9/,]|Space|Enter|Up|Down|Left|Right|F(?:[1-9]|1[0-2]))$/;
  let registered;
  let owner;
  function clear() {
    if (registered) globalShortcut.unregister(registered);
    registered = undefined;
    owner = undefined;
  }

  // Check a proposed binding at edit time, even outside a call.
  ipcMain.handle('chatto:mute-shortcut:available', (event, chord) => {
    if (!isTrusted(event) || typeof chord !== 'string' || !validChord.test(chord)) return false;
    // Wayland portal requests finish asynchronously. A temporary registration
    // can prompt for consent or race the call's real registration.
    if (wayland || registered === chord) return true;
    try {
      if (!globalShortcut.register(chord, () => {})) return false;
      globalShortcut.unregister(chord);
      return true;
    } catch {
      return false;
    }
  });

  ipcMain.handle('chatto:mute-shortcut:set', (event, chord, token) => {
    if (!isTrusted(event)) return false;
    if (chord === '') {
      if (owner === token) clear();
      return true;
    }
    clear();
    // Accept only modified chords offered by the shortcut editor.
    if (typeof chord !== 'string' || typeof token !== 'string' || !validChord.test(chord))
      return false;
    try {
      if (
        !globalShortcut.register(chord, () => {
          if (owner === token && registered === chord && !contents.isDestroyed())
            contents.send('chatto:mute-shortcut:pressed', token);
        })
      )
        return false;
      registered = chord;
      owner = token;
      return true;
    } catch {
      return false;
    }
  });
  contents.on('render-process-gone', clear);
  contents.on('destroyed', clear);
  contents.on('did-start-navigation', ({ isSameDocument, isMainFrame }) => {
    if (isMainFrame && !isSameDocument) clear();
  });
}
