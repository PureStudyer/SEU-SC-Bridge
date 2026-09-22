param([ValidateSet('amd64','arm64')][string]$Arch='amd64',[string]$Version='1.0.1',[switch]$CLI)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
 & go run github.com/tc-hib/go-winres@v0.3.3 make --in packaging/windows/winres.json --out cmd/seusc/rsrc --arch $Arch --product-version $Version --file-version $Version
 if($LASTEXITCODE -ne 0){throw 'Windows resource generation failed'}
 $env:GOOS='windows'; $env:GOARCH=$Arch; $env:CGO_ENABLED='0'
 $out=Join-Path $root "dist/windows-$Arch"
 New-Item -ItemType Directory -Force -Path $out | Out-Null
 $tags='desktop,production'; if($CLI){$tags=''}
 & go build -trimpath -tags $tags -ldflags "-s -w -X github.com/PureStudyer/SEU-SC-Bridge/internal/agent.Version=$Version" -o "$out/seusc.exe" ./cmd/seusc
 if($LASTEXITCODE -ne 0){throw 'Go build failed'}
 if($env:WINDOWS_SIGNING_CERTIFICATE_BASE64){
  & "$root/scripts/sign-windows.ps1" -Path "$out/seusc.exe"
  if($LASTEXITCODE -ne 0){throw 'Windows executable signing failed'}
 } else {
  Write-Warning 'WINDOWS_SIGNING_CERTIFICATE_BASE64 is not set; output is unsigned.'
 }
 Copy-Item -LiteralPath README.md -Destination $out
 Write-Host "Built $out/seusc.exe"
} finally {Pop-Location}
