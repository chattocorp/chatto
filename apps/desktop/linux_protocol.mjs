import { execFile } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';

/** Register the running AppImage or extracted tarball for the browser's return trip. */
export async function registerLinuxCallback(packageName) {
  const directory = path.join(
    process.env.XDG_DATA_HOME || path.join(homedir(), '.local/share'),
    'applications'
  );
  const executable = process.env.APPIMAGE || process.execPath;
  if (!path.isAbsolute(executable) || /[\r\n]/.test(executable))
    throw new Error('Invalid Desktop executable.');
  // Escape Exec field codes, quoted arguments, and desktop-entry string escapes.
  const quoted = executable
    .replace(/%/g, '%%')
    .replace(/(["`$\\])/g, '\\$1')
    .replace(/\\/g, '\\\\');
  const name = packageName + '-oauth.desktop';
  await mkdir(directory, { recursive: true });
  await writeFile(
    path.join(directory, name),
    [
      '[Desktop Entry]',
      'Type=Application',
      'Name=Chatto Desktop sign-in',
      'NoDisplay=true',
      'MimeType=x-scheme-handler/chatto;',
      'Exec="' + quoted + '" %u',
      ''
    ].join('\n')
  );
  const xdgMime = promisify(execFile);
  await xdgMime('xdg-mime', ['default', name, 'x-scheme-handler/chatto']);
  const { stdout } = await xdgMime('xdg-mime', ['query', 'default', 'x-scheme-handler/chatto']);
  return stdout.trim() === name;
}
