/** Map release versions to the desktop feeds. RC releases use the Beta feed. */
export function desktopUpdateChannel(version) {
  if (/^\d+\.\d+\.\d+$/.test(version)) return 'latest';
  if (/^\d+\.\d+\.\d+-(?:beta|rc)\.\d+$/.test(version)) return 'beta';
  return undefined;
}
