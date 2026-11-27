/** Compile the small system hotkey listener for source launches and distribution. */
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));

export function buildPushToTalk(architecture = process.arch) {
  const source = path.join(root, 'native/push-to-talk');
  const folder = path.join(root, 'dist/native');
  mkdirSync(folder, { recursive: true });
  const binary = path.join(
    folder,
    process.platform === 'win32' ? 'push-to-talk.exe' : 'push-to-talk'
  );
  if (process.platform === 'darwin') {
    const archs =
      architecture === 'universal'
        ? ['arm64', 'x86_64']
        : [architecture === 'x64' ? 'x86_64' : architecture];
    execFileSync('xcrun', [
      'clang',
      ...archs.flatMap((arch) => ['-arch', arch]),
      '-mmacosx-version-min=12.0',
      '-fblocks',
      path.join(source, 'macos.m'),
      '-framework',
      'AppKit',
      '-framework',
      'Carbon',
      '-o',
      binary
    ]);
  } else if (process.platform === 'win32') {
    execFileSync('go', [
      'build',
      '-trimpath',
      '-ldflags=-s -w',
      '-o',
      binary,
      path.join(source, 'windows.go')
    ]);
  } else {
    const flags = execFileSync('pkg-config', ['--cflags', '--libs', 'gio-unix-2.0', 'x11'], {
      encoding: 'utf8'
    })
      .trim()
      .split(/\s+/);
    execFileSync('cc', [path.join(source, 'linux.c'), ...flags, '-o', binary]);
  }
  return binary;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) buildPushToTalk();
