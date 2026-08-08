/** Convert a SemVer release into the numeric build version expected by packagers. */
export function releaseBuildVersion(version) {
  const match = /^(\d+)\.(\d+)\.(\d+)(?:-[^.]+\.(\d+))?$/.exec(version);
  if (!match) throw new TypeError(`Unsupported desktop version: ${version}`);
  return match.slice(1).filter(Boolean).join(".");
}
