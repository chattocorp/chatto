/** Sign and notarise the outer distribution container before publication. */
import { execFileSync } from 'node:child_process';
import { macOSDistributionOptions } from './macos-signing.mjs';
import { stapleMacOSArtifact, submitMacOSArtifact } from './macos-notarization.mjs';

/** Fail closed on missing credentials, rejection, or failed ticket validation. */
export async function signMacOSDmg(file, environment, run = execFileSync) {
  const { osxSign, osxNotarize } = macOSDistributionOptions('darwin', environment);
  if (!osxNotarize) {
    throw new Error('DMG release signing requires all macOS notarisation credentials.');
  }
  const inherited = { stdio: 'inherit' };
  run('codesign', ['--sign', osxSign.identity, '--timestamp', file], inherited);
  submitMacOSArtifact(file, osxNotarize, run);
  await stapleMacOSArtifact(file, run);
  run('codesign', ['--verify', '--strict', '--verbose=2', file], inherited);
  run(
    'spctl',
    ['--assess', '--type', 'open', '--context', 'context:primary-signature', '--verbose=4', file],
    inherited
  );
}
