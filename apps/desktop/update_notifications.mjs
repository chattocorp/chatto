/** Native feedback that must outlive a menu command or an open Updates page. */
export function createUpdateNotifier({ Notification, dialog }) {
  return async (state, version, isManual) => {
    if (state === 'install-error') {
      await dialog.showMessageBox({
        type: 'error',
        message: 'Chatto could not install this update. Check for updates again.'
      });
    } else if (state === 'ready' && !isManual && Notification.isSupported()) {
      new Notification({
        title: 'Chatto update ready',
        body: `Chatto ${version} is ready. Open Updates preferences to restart and install it.`
      }).show();
    }
  };
}
