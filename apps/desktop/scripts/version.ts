export interface MacOSVersions {
  bundleVersion: string;
  shortVersion: string;
}

/** Convert the desktop SemVer into values accepted by macOS bundle metadata. */
export function macOSVersions(version: string): MacOSVersions {
  const match =
    /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/
      .exec(
        version,
      );
  if (!match) throw new TypeError(`Invalid desktop version: ${version}`);

  const shortVersion = `${match[1]}.${match[2]}.${match[3]}`;
  const prerelease = match[4];
  if (!prerelease) {
    return { shortVersion, bundleVersion: shortVersion };
  }

  const [channel, rawBuild = "1"] = prerelease.split(".");
  const build = Number.parseInt(rawBuild, 10);
  const safeBuild = Number.isInteger(build) && build >= 1 && build <= 255
    ? build
    : 1;
  const suffix = channel === "alpha"
    ? "a"
    : channel === "beta"
    ? "b"
    : channel === "rc"
    ? "fc"
    : "d";
  return {
    shortVersion,
    bundleVersion: `${shortVersion}${suffix}${safeBuild}`,
  };
}
