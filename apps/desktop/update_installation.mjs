/** Detect the Windows NSIS installation required for automatic updates. */
import { existsSync } from 'node:fs';
import path from 'node:path';

/** ZIP copies have no uninstaller. NSIS names it after the configured executable. */
export function hasWindowsUninstaller(execPath) {
  return existsSync(path.join(path.dirname(execPath), 'Uninstall chatto-desktop.exe'));
}
