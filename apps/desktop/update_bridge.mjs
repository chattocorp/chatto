/** Narrow update IPC. Only the trusted main frame can read state or run commands. */
export function configureUpdateBridge({ ipcMain, controller, isTrusted, send }) {
  for (const [name, action] of Object.entries({
    status: (updates) => updates?.snapshot() ?? null,
    check: (updates) => updates?.check(true),
    preference: (updates, request) => updates?.setPreference(request?.key, request?.value),
    install: (updates) => updates?.install()
  })) {
    ipcMain.handle(`chatto:updates:${name}`, async (event, request) => {
      if (!isTrusted(event)) throw new Error('Updates are available only to the Chatto renderer.');
      const updates = await controller;
      // Setup can outlive the sending document.
      if (!isTrusted(event)) throw new Error('The update request expired.');
      try {
        return await action(updates, request);
      } catch {
        throw new Error('The update operation failed.');
      }
    });
  }
  void controller.then((updates) => updates?.subscribe(send));
}
