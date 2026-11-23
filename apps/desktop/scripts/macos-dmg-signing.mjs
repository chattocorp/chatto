/** Sign and notarise the outer distribution container before publication. */
import { execFileSync } from 'node:child_process';
import { macOSDistributionOptions } from './macos-signing.mjs';
import { stapleMacOSArtifact, submitMacOSArtifact } from './macos-notarization.mjs';

/**
 * Notarise the disk image and the app inside it with one Apple submission.
 *
 * `app` must be the same signed bundle that the disk image contains. Apple
 * issues a ticket for each nested code item, so the bundle can receive its own
 * ticket for the updater ZIP without a second submission. Fails closed on
 * missing credentials, rejection, or failed ticket or signature validation.
 */
export async function signMacOSDmg(file, app, environment, run = execFileSync) {
  const { osxSign, osxNotarize } = macOSDistributionOptions('darwin', environment);
  if (!osxNotarize) {
    throw new Error('DMG release signing requires all macOS notarisation credentials.');
  }
  const inherited = { stdio: 'inherit' };
  run('codesign', ['--sign', osxSign.identity, '--timestamp', file], inherited);
  submitMacOSArtifact(file, osxNotarize, run);
  await stapleMacOSArtifact(file, run);
  // Staple the bundle only after the disk image exists. The disk image keeps
  // the unstapled copy; its own stapled ticket covers that copy.
  await stapleMacOSArtifact(app, run);
  run('codesign', ['--verify', '--strict', '--verbose=2', file], inherited);
  run(
    'spctl',
    ['--assess', '--type', 'open', '--context', 'context:primary-signature', '--verbose=4', file],
    inherited
  );
  run('spctl', ['--assess', '--type', 'execute', '--verbose=4', app], inherited);
}
