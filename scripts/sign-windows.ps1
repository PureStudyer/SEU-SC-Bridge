param([Parameter(Mandatory=$true)][string]$Path)
$ErrorActionPreference='Stop'
if(-not $env:WINDOWS_SIGNING_CERTIFICATE_BASE64){throw 'WINDOWS_SIGNING_CERTIFICATE_BASE64 is required for a signed Windows artifact'}
if(-not $env:WINDOWS_SIGNING_CERTIFICATE_PASSWORD){throw 'WINDOWS_SIGNING_CERTIFICATE_PASSWORD is required for a signed Windows artifact'}
$signTool=(Get-Command signtool.exe -ErrorAction SilentlyContinue).Source
if(-not $signTool){
 $signTool=(Get-ChildItem -Path "$env:ProgramFiles(x86)\Windows Kits\10\bin" -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue | Where-Object {$_.FullName -match '\\x64\\signtool\.exe$'} | Sort-Object FullName | Select-Object -Last 1).FullName
}
if(-not $signTool){throw 'signtool.exe was not found'}
$certPath=Join-Path ([IO.Path]::GetTempPath()) ("seusc-signing-{0}.pfx" -f $PID)
try {
 [IO.File]::WriteAllBytes($certPath,[Convert]::FromBase64String($env:WINDOWS_SIGNING_CERTIFICATE_BASE64))
 $timestamp=$env:WINDOWS_TIMESTAMP_URL
 if(-not $timestamp){$timestamp='http://timestamp.digicert.com'}
 & $signTool sign /fd SHA256 /td SHA256 /tr $timestamp /f $certPath /p $env:WINDOWS_SIGNING_CERTIFICATE_PASSWORD $Path
 if($LASTEXITCODE -ne 0){throw "signtool failed for $Path"}
} finally {
 Remove-Item -LiteralPath $certPath -Force -ErrorAction SilentlyContinue
}
