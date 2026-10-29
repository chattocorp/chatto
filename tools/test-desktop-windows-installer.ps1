# Run only on disposable CI runners: NSIS writes the current user's install registry.
param([Parameter(Mandatory)][string]$Installer)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or -not $env:RUNNER_TEMP) {
    throw 'Installer smoke tests require a disposable GitHub Actions Windows runner.'
}
$Installer = (Resolve-Path -LiteralPath $Installer).Path
$installDir = Join-Path $env:RUNNER_TEMP "chatto-installer-test-$([guid]::NewGuid())"
$profileDir = Join-Path $env:APPDATA 'Chatto Desktop'
$marker = Join-Path $profileDir "installer-test-$([guid]::NewGuid()).txt"
$shortcut = Join-Path $env:APPDATA 'Microsoft/Windows/Start Menu/Programs/Chatto Desktop.lnk'

function Run-Installer {
    $process = Start-Process -FilePath $Installer -ArgumentList "/S /D=$installDir" -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Installer failed: $($process.ExitCode)" }
    if (-not (Test-Path "$installDir/chatto-desktop.exe")) { throw 'Application missing.' }
    if (-not (Test-Path $shortcut)) { throw 'Start menu shortcut missing.' }
    $registration = @(Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' |
        Where-Object { $_.DisplayName -like 'Chatto Desktop*' -and
            ([string]$_.UninstallString).StartsWith('"' + $installDir + '\') })
    if ($registration.Count -ne 1) { throw 'Expected one per-user uninstall registration.' }
}

try {
    New-Item -ItemType Directory -Force $profileDir | Out-Null
    Set-Content -LiteralPath $marker -Value 'preserve profile'
    Run-Installer
    # A second installation must replace obsolete application files, not merge them.
    Set-Content -LiteralPath "$installDir/obsolete-installer-test.txt" -Value 'old version'
    Run-Installer
    if (Test-Path "$installDir/obsolete-installer-test.txt") { throw 'Reinstall left obsolete files.' }
    if ((Get-Content -LiteralPath $marker) -ne 'preserve profile') { throw 'Reinstall changed profile.' }
    $uninstallers = @(Get-ChildItem -LiteralPath $installDir -Filter 'Uninstall*.exe')
    if ($uninstallers.Count -ne 1) { throw 'Expected one uninstaller.' }
    # _?= avoids spawning a detached temporary uninstaller, so -Wait covers removal.
    $process = Start-Process -FilePath $uninstallers[0].FullName -ArgumentList "/S _?=$installDir" -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Uninstaller failed: $($process.ExitCode)" }
    if (Test-Path "$installDir/chatto-desktop.exe") { throw 'Uninstall left the application.' }
    if (Test-Path $shortcut) { throw 'Uninstall left the shortcut.' }
    if ((Get-Content -LiteralPath $marker) -ne 'preserve profile') { throw 'Uninstall changed profile.' }
    Write-Host 'Verified install, reinstall, per-user registration, shortcut, uninstall, and profile preservation.'
} finally {
    Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $installDir -Recurse -Force -ErrorAction SilentlyContinue
}
