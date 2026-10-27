# Sign each generated NSIS executable before it is embedded or published.
param([Parameter(Mandatory)][string]$File)
$ErrorActionPreference = 'Stop'

foreach ($name in @('CHATTO_WINDOWS_SIGNING_ENDPOINT', 'CHATTO_WINDOWS_SIGNING_ACCOUNT_NAME',
    'CHATTO_WINDOWS_CERTIFICATE_PROFILE_NAME', 'CHATTO_WINDOWS_EXPECTED_PUBLISHER')) {
    if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))) {
        throw "Missing Windows signing setting: $name"
    }
}
Import-Module ArtifactSigning -ErrorAction Stop
Invoke-ArtifactSigning -Endpoint $env:CHATTO_WINDOWS_SIGNING_ENDPOINT `
    -CodeSigningAccountName $env:CHATTO_WINDOWS_SIGNING_ACCOUNT_NAME `
    -CertificateProfileName $env:CHATTO_WINDOWS_CERTIFICATE_PROFILE_NAME `
    -Files $File -FileDigest SHA256 -TimestampDigest SHA256 `
    -TimestampRfc3161 'http://timestamp.acs.microsoft.com' `
    -ExcludeEnvironmentCredential:$true -ExcludeWorkloadIdentityCredential:$true `
    -ExcludeManagedIdentityCredential:$true -ExcludeAzureCliCredential:$false `
    -Description 'Chatto Desktop' -DescriptionUrl 'https://chatto.run'

$signature = Get-AuthenticodeSignature -LiteralPath $File
if ($signature.Status -ne 'Valid' -or $null -eq $signature.TimeStamperCertificate -or
    -not [string]::Equals($signature.SignerCertificate.Subject,
        $env:CHATTO_WINDOWS_EXPECTED_PUBLISHER, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Invalid, untimestamped, or unexpected publisher signature on $File"
}
