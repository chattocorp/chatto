/** Keep macOS window closing separate from app quitting; the Dock reopens the retained renderer. */
export function configureBackgroundWindow({
  app,
  autoUpdater,
  window,
  platform = process.platform
}) {
  let quitting = false;
  const open = () => {
    if (platform === 'darwin') app.show();
    if (window.isMinimized()) window.restore();
    window.show();
    window.focus();
  };
  window.on('close', (event) => {
    if (quitting || platform !== 'darwin') return;
    event.preventDefault();
    window.hide();
  });
  app.on('before-quit', () => {
    quitting = true;
  });
  // Native update installation closes windows before app emits before-quit.
  // Listen to Electron's updater, which electron-updater uses on macOS.
  autoUpdater.on('before-quit-for-update', () => {
    quitting = true;
  });
  app.on('activate', open);
  app.on('second-instance', open);
  return { open };
}
