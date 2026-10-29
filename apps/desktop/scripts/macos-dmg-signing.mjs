/** Sign and notarise the outer distribution container before publication. */
import { execFileSync } from 'node:child_process';
import { macOSDistributionOptions } from './macos-signing.mjs';

/** Fail closed on missing credentials, rejection, or failed ticket validation. */
export function signMacOSDmg(file, environment, run = execFileSync) {
  const { osxSign, osxNotarize } = macOSDistributionOptions('darwin', environment);
  if (!osxNotarize) {
    throw new Error('DMG release signing requires all macOS notarisation credentials.');
  }
  const inherited = { stdio: 'inherit' };
  run('codesign', ['--sign', osxSign.identity, '--timestamp', file], inherited);
  // notarytool can exit successfully for a rejected submission. Check its status.
  const submission = JSON.parse(
    run(
      'xcrun',
      [
        'notarytool',
        'submit',
        file,
        '--key',
        osxNotarize.appleApiKey,
        '--key-id',
        osxNotarize.appleApiKeyId,
        '--issuer',
        osxNotarize.appleApiIssuer,
        '--wait',
        '--output-format',
        'json'
      ],
      { encoding: 'utf8' }
    )
  );
  console.log(`DMG notarisation ${submission.id}: ${submission.status}`);
  if (submission.status !== 'Accepted') {
    throw new Error(`DMG notarisation was not accepted: ${submission.id}`);
  }
  run('xcrun', ['stapler', 'staple', file], inherited);
  run('xcrun', ['stapler', 'validate', file], inherited);
  run('codesign', ['--verify', '--strict', '--verbose=2', file], inherited);
  run(
    'spctl',
    ['--assess', '--type', 'open', '--context', 'context:primary-signature', '--verbose=4', file],
    inherited
  );
}
