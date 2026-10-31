/** Generate updater manifests from the exact signed files collected for publication. */
import { createHash } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { stat, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

/** All update-capable platforms must be present before any manifest is written. */
export async function writeUpdateManifests(
  directory,
  version,
  releaseDate = new Date().toISOString()
) {
  if (!/^\d+\.\d+\.\d+(?:-beta\.\d+)?$/.test(version)) {
    throw new Error('Desktop updates require a stable or beta release version.');
  }
  const channel = version.includes('-') ? 'beta' : 'latest';
  const artifacts = [
    [`${channel}-mac.yml`, `chatto-desktop_${version}_macOS_ARM64.zip`],
    [`${channel}.yml`, `chatto-desktop_${version}_Windows_X64_Setup.exe`],
    [`${channel}-linux.yml`, `chatto-desktop_${version}_Linux_X64.AppImage`],
    [`${channel}-linux-arm64.yml`, `chatto-desktop_${version}_Linux_ARM64.AppImage`]
  ];
  const manifests = await Promise.all(
    artifacts.map(async ([manifest, file]) => {
      const fullPath = path.join(directory, file);
      const { size } = await stat(fullPath);
      if (size === 0) throw new Error(`Empty update artifact: ${file}`);
      const hash = createHash('sha512');
      for await (const chunk of createReadStream(fullPath)) hash.update(chunk);
      const sha512 = hash.digest('base64');
      return [
        manifest,
        {
          version,
          files: [{ url: file, sha512, size }],
          path: file,
          sha512,
          releaseDate,
          rolloutHours: 36
        }
      ];
    })
  );
  for (const [name, manifest] of manifests) {
    await writeFile(path.join(directory, name), JSON.stringify(manifest, null, 2) + '\n');
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [directory, version] = process.argv.slice(2);
  if (!directory || !version) throw new Error('Usage: update-manifests.mjs DIRECTORY VERSION');
  await writeUpdateManifests(directory, version);
}
