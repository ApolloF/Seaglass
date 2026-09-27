# Signs a file with Authenticode when a code-signing certificate is set up,
# and does nothing otherwise (unsigned builds still work; see docs/SIGNING.md).
#
#   SIGN_PFX_BASE64 + SIGN_PFX_PASSWORD   a .pfx certificate, base64-encoded
#   SIGN_THUMBPRINT                       a certificate in the current user's store
#                                         (for example on a hardware token)
#   SIGN_TIMESTAMP_URL                    RFC 3161 timestamp server (default DigiCert)
param([Parameter(Mandatory = $true)][string]$File)
$ErrorActionPreference = 'Stop'

if (-not $env:SIGN_PFX_BASE64 -and -not $env:SIGN_THUMBPRINT) {
    Write-Host "Not signing $File (no certificate configured)"
    exit 0
}

$signtool = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" -ErrorAction SilentlyContinue |
    Sort-Object FullName -Descending | Select-Object -First 1
if (-not $signtool) { throw 'signtool.exe not found (install the Windows SDK)' }

$ts = if ($env:SIGN_TIMESTAMP_URL) { $env:SIGN_TIMESTAMP_URL } else { 'http://timestamp.digicert.com' }
$signArgs = @('sign', '/fd', 'sha256', '/tr', $ts, '/td', 'sha256', '/d', 'Seaglass', '/du', 'https://github.com/ApolloF/Seaglass')
$pfx = $null
try {
    if ($env:SIGN_PFX_BASE64) {
        $pfx = Join-Path ([IO.Path]::GetTempPath()) ("wl-" + [Guid]::NewGuid() + '.pfx')
        [IO.File]::WriteAllBytes($pfx, [Convert]::FromBase64String($env:SIGN_PFX_BASE64))
        $signArgs += @('/f', $pfx, '/p', $env:SIGN_PFX_PASSWORD)
    } else {
        $signArgs += @('/sha1', $env:SIGN_THUMBPRINT)
    }
    & $signtool.FullName @signArgs $File
    if ($LASTEXITCODE -ne 0) { throw "signtool failed ($LASTEXITCODE)" }
    & $signtool.FullName verify /pa /q $File
    if ($LASTEXITCODE -ne 0) { throw "signature on $File doesn't verify" }
    Write-Host "Signed $File"
} finally {
    if ($pfx) { Remove-Item $pfx -Force -ErrorAction SilentlyContinue }
}
