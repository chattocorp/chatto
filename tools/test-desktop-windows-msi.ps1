# Run only on disposable CI runners: this test changes machine-wide MSI state.
param(
    [Parameter(Mandatory)][string]$Installer,
    [Parameter(Mandatory)][string]$PreviousInstaller
)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or -not $env:RUNNER_TEMP) {
    throw 'MSI smoke tests require a disposable GitHub Actions Windows runner.'
}
$Installer = (Resolve-Path -LiteralPath $Installer).Path
$PreviousInstaller = (Resolve-Path -LiteralPath $PreviousInstaller).Path
$testId = [guid]::NewGuid()
$installDir = Join-Path $env:RUNNER_TEMP "chatto-msi-test-$testId"
$profileDir = Join-Path $env:APPDATA 'Chatto Desktop'
$marker = Join-Path $profileDir "msi-test-$testId.txt"
$shortcut = Join-Path $env:ProgramData 'Microsoft/Windows/Start Menu/Programs/Chatto.lnk'
$logDir = Join-Path $PSScriptRoot '../.context/msi-test'
New-Item -ItemType Directory -Force $logDir | Out-Null

function Run-Msi([string]$Arguments, [string]$Step) {
    $log = Join-Path $logDir "$testId-$Step.log"
    $process = Start-Process msiexec.exe -ArgumentList "$Arguments /qn /norestart /l*v `"$log`"" -Wait -PassThru
    if ($process.ExitCode -notin @(0, 3010)) {
        throw "MSI $Step failed with exit code $($process.ExitCode). See $log"
    }
}

function Get-Registration {
    return @(Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' |
        Where-Object { $_.DisplayName -eq 'Chatto' -and $_.WindowsInstaller -eq 1 })
}

try {
    New-Item -ItemType Directory -Force $profileDir | Out-Null
    Set-Content -LiteralPath $marker -Value 'preserve profile'
    Run-Msi "/i `"$PreviousInstaller`" APPLICATIONFOLDER=`"$installDir`"" 'install-previous'
    $previous = Get-Registration
    if ($previous.Count -ne 1) { throw 'Expected one previous MSI registration.' }
    Run-Msi "/i `"$Installer`" APPLICATIONFOLDER=`"$installDir`"" 'install'
    if (-not (Test-Path "$installDir/chatto-desktop.exe")) { throw 'Application missing.' }
    if (-not (Test-Path $shortcut)) { throw 'Machine-wide Start menu shortcut missing.' }
    if ((Get-Registration).Count -ne 1) { throw 'Expected one machine-wide MSI registration.' }
    $current = Get-Registration
    if ([version]$current[0].DisplayVersion -le [version]$previous[0].DisplayVersion) {
        throw 'Upgrade did not increase the installed MSI version.'
    }
    if ($current[0].PSChildName -eq $previous[0].PSChildName) { throw 'Upgrade did not replace the previous product.' }
    if ((Get-Content -LiteralPath $marker) -ne 'preserve profile') { throw 'Upgrade changed profile.' }
    if (Test-Path "$installDir/Uninstall chatto-desktop.exe") {
        throw 'MSI must not enable the NSIS updater.'
    }

    # Exercise Windows Installer repair against a missing installed file.
    Remove-Item -LiteralPath "$installDir/chatto-desktop.exe" -Force
    Run-Msi "/fa `"$Installer`"" 'repair'
    if (-not (Test-Path "$installDir/chatto-desktop.exe")) { throw 'Repair did not restore the application.' }
    if ((Get-Content -LiteralPath $marker) -ne 'preserve profile') { throw 'Repair changed profile.' }

    Run-Msi "/x `"$Installer`"" 'uninstall'
    if (Test-Path "$installDir/chatto-desktop.exe") { throw 'Uninstall left the application.' }
    if (Test-Path $shortcut) { throw 'Uninstall left the shortcut.' }
    if ((Get-Registration).Count -ne 0) { throw 'Uninstall left MSI registration.' }
    if ((Get-Content -LiteralPath $marker) -ne 'preserve profile') { throw 'Uninstall changed profile.' }
    Write-Host 'Verified MSI install, upgrade, machine-wide registration, shortcut, repair, uninstall, and profile preservation.'
} finally {
    # Remove MSI state even if an assertion fails, before deleting test files.
    if ((Get-Registration).Count -gt 0) {
        foreach ($registration in (Get-Registration)) {
            Run-Msi "/x $($registration.PSChildName)" 'cleanup'
        }
    }
    Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $installDir -Recurse -Force -ErrorAction SilentlyContinue
}
