/** macOS password presentation. The password never appears in process arguments or logs. */
import { execFile } from 'node:child_process';

/** Ask through a system secure field until GPUix exposes native password masking. */
export function promptPassword(signal?: AbortSignal): Promise<string | null> {
  if (process.platform !== 'darwin')
    throw new Error('Secure password entry currently requires macOS');
  const script =
    'text returned of (display dialog "Enter your Chatto password for the server shown in Chatto GPUix." default answer "" with hidden answer buttons {"Cancel", "Sign in"} default button "Sign in" with title "Chatto GPUix")';
  return new Promise((resolve, reject) => {
    execFile(
      '/usr/bin/osascript',
      ['-e', script],
      { timeout: 120_000, maxBuffer: 16_384, signal },
      (error, stdout, stderr) => {
        if (error) {
          if (stderr.includes('(-128)')) resolve(null);
          else reject(new Error('The secure password dialog could not be opened'));
        } else resolve(stdout.replace(/\r?\n$/, ''));
      }
    );
  });
}
