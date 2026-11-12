/** Resolve the target shared by app packaging and DMG creation. */
export function desktopArchitecture(platform, hostArchitecture, environment) {
  const architecture = environment.CHATTO_DESKTOP_ARCH || hostArchitecture;
  if (platform === 'darwin') {
    if (!['x64', 'arm64', 'universal'].includes(architecture)) {
      throw new Error(`Unsupported macOS desktop architecture: ${architecture}`);
    }
  } else if (architecture !== hostArchitecture) {
    throw new Error('CHATTO_DESKTOP_ARCH only supports cross-compilation on macOS.');
  }
  return architecture;
}
