/** Own native alerts for the current renderer document. Never accept navigation URLs over IPC. */
export function configureDesktopNotifications({
  ipcMain,
  Notification,
  getWindow,
  isTrusted,
  open
}) {
  const alerts = new Map();
  const clear = (ids = [...alerts.keys()]) => {
    for (const id of ids) {
      const alert = alerts.get(id);
      alerts.delete(id);
      alert?.close();
    }
  };
  ipcMain.handle('chatto:notifications:show', (event, request) => {
    if (!isTrusted(event)) throw new Error('Untrusted notification request.');
    if (!Notification.isSupported()) throw new Error('Native notifications are unavailable.');
    if (
      !request ||
      typeof request.id !== 'string' ||
      request.id.length > 256 ||
      !request.id ||
      typeof request.title !== 'string' ||
      request.title.length > 256 ||
      typeof request.body !== 'string' ||
      request.body.length > 1024
    )
      throw new Error('Invalid notification request.');
    if (alerts.has(request.id)) return;
    if (alerts.size >= 64) clear([alerts.keys().next().value]);
    const alert = new Notification({ title: request.title, body: request.body, silent: true });
    alerts.set(request.id, alert);
    alert.on('click', () => {
      if (alerts.get(request.id) !== alert) return;
      // Route before focus can mark the currently visible conversation read and clear its target.
      getWindow()?.webContents.send('chatto:notifications:click', request.id);
      open();
      clear([request.id]);
    });
    alert.on('close', () => {
      if (alerts.get(request.id) === alert) alerts.delete(request.id);
    });
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        clear([request.id]);
        reject(new Error('Notification delivery was not confirmed.'));
      }, 5000);
      alert.once('show', () => {
        clearTimeout(timer);
        resolve();
      });
      alert.once('failed', () => {
        clearTimeout(timer);
        clear([request.id]);
        reject(new Error('The operating system could not show the notification.'));
      });
      try {
        alert.show();
      } catch {
        clearTimeout(timer);
        clear([request.id]);
        reject(new Error('The operating system could not show the notification.'));
      }
    });
  });
  ipcMain.on('chatto:notifications:clear', (event, ids) => {
    if (!isTrusted(event) || !Array.isArray(ids) || ids.length > 64) return;
    clear(ids.filter((id) => typeof id === 'string'));
  });
  return { clear };
}
