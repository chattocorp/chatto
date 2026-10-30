/** Keep development installs separate from the signed release and its permissions. */
export function desktopBuildProfile(variant = 'release') {
  if (!['release', 'development'].includes(variant)) {
    throw new Error('CHATTO_DESKTOP_BUILD must be release or development.');
  }
  const development = variant === 'development';
  return {
    variant,
    productName: development ? 'Chatto Dev' : 'Chatto',
    // Keep existing sessions and preferences when the display name changes.
    userDataName: development ? 'Chatto Desktop Dev' : 'Chatto Desktop',
    packageName: development ? 'chatto-desktop-dev' : 'chatto-desktop',
    bundleIdentifier: development ? 'run.chatto.desktop.dev' : 'run.chatto.desktop'
  };
}
