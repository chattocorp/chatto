import assert from 'node:assert/strict';
import test from 'node:test';
import { windowsInstallerConfig } from './windows-installer-config.mjs';

test('stable and beta installers share identity, executable, and profile-preserving policy', () => {
  const beta = windowsInstallerConfig('0.5.0-beta.21');
  const stable = windowsInstallerConfig('0.5.0');
  assert.equal(beta.appId, stable.appId);
  assert.equal(beta.appId, 'run.chatto.desktop');
  assert.equal(beta.executableName, 'chatto-desktop');
  assert.equal(beta.productName, 'Chatto Desktop');
  assert.equal(beta.buildVersion, '0.5.0.21');
  assert.equal(beta.nsis.oneClick, true);
  assert.equal(beta.nsis.perMachine, false);
  assert.equal(beta.nsis.packElevateHelper, false);
  assert.equal(beta.nsis.deleteAppDataOnUninstall, false);
  assert.equal(beta.publish, null);
  assert.equal(beta.nsis.differentialPackage, false);
  assert.equal(beta.nsis.artifactName, 'chatto-desktop_0.5.0-beta.21_Windows_X64_Setup.exe');
});

test('release signing requires the provided hook and SHA-256; CI is explicitly unsigned', () => {
  const sign = async () => {};
  const release = windowsInstallerConfig('0.5.0', sign);
  assert.equal(release.forceCodeSigning, true);
  assert.equal(release.win.signExecutable, true);
  assert.equal(release.win.signtoolOptions.sign, sign);
  assert.deepEqual(release.win.signtoolOptions.signingHashAlgorithms, ['sha256']);
  const ci = windowsInstallerConfig('0.5.0');
  assert.equal(ci.forceCodeSigning, false);
  assert.equal(ci.win.signExecutable, false);
});
