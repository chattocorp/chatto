import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { windowsInstallerConfig } from './windows-installer-config.mjs';

test(
  'installer signing binds credential switches without positional file arguments',
  {
    skip: process.platform !== 'win32'
  },
  () => {
    // Use PowerShell's real parameter binder but replace signing and certificate
    // lookup. This exercises the release hook without credentials or a network.
    execFileSync(
      'pwsh',
      [
        '-NoProfile',
        '-NonInteractive',
        '-Command',
        `
$ErrorActionPreference = 'Stop'
function Import-Module { param($Name, $ErrorAction) }
function Invoke-ArtifactSigning {
  [CmdletBinding(PositionalBinding=$false)]
  param(
    [string]$Endpoint, [string]$CodeSigningAccountName,
    [string]$CertificateProfileName, [string]$Files,
    [string]$FileDigest, [string]$TimestampDigest, [string]$TimestampRfc3161,
    [switch]$ExcludeEnvironmentCredential, [switch]$ExcludeWorkloadIdentityCredential,
    [switch]$ExcludeManagedIdentityCredential, [switch]$ExcludeAzureCliCredential,
    [string]$Description, [string]$DescriptionUrl
  )
  if (-not $ExcludeEnvironmentCredential -or -not $ExcludeWorkloadIdentityCredential -or
      -not $ExcludeManagedIdentityCredential -or $ExcludeAzureCliCredential) {
    throw 'Incorrect credential selection'
  }
  if ($Files -ne 'installer.exe') { throw 'Incorrect signing target' }
  $script:signed = $true
}
function Get-AuthenticodeSignature {
  param($LiteralPath)
  if (-not $script:signed) { throw 'Signing was not called' }
  return @{ Status = 'Valid'; TimeStamperCertificate = @{};
    SignerCertificate = @{ Subject = 'test-publisher' } }
}
& $env:TEST_SIGNING_SCRIPT -File 'installer.exe'
`
      ],
      {
        stdio: 'pipe',
        env: {
          ...process.env,
          TEST_SIGNING_SCRIPT: fileURLToPath(
            new URL('./sign-windows-installer.ps1', import.meta.url)
          ),
          CHATTO_WINDOWS_SIGNING_ENDPOINT: 'https://signing.invalid',
          CHATTO_WINDOWS_SIGNING_ACCOUNT_NAME: 'test-account',
          CHATTO_WINDOWS_CERTIFICATE_PROFILE_NAME: 'test-profile',
          CHATTO_WINDOWS_EXPECTED_PUBLISHER: 'test-publisher'
        }
      }
    );
  }
);

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
