# Builds dist\Switaculous-<version>-<arch>.msi. Needs Go and WiX 5:
#   dotnet tool install --global wix --version 5.0.2
#   wix extension add -g WixToolset.UI.wixext/5.0.2
# install.ps1 in the repository root sets all of this up for you.
param(
    [string]$Version = "1.0.0",
    [string]$Arch = "x64",
    [string]$Wix = "wix"
)
$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$msiFolder = Join-Path $root "dist\msi-$Arch"
$binary = Join-Path $msiFolder "switaculous.exe"
New-Item -ItemType Directory -Force -Path $msiFolder | Out-Null

# The installed copy is built as a Windows app, so starting it from the Start menu
# opens the editor without a console window. Quit is in the editor's menu.
$goArch = if ($Arch -eq "arm64") { "arm64" } else { "amd64" }
# The program icon is compiled in from switaculous.ico, so no binary files live in the repository.
& go -C $root tool rsrc -ico packaging\windows\switaculous.ico -arch $goArch -o "rsrc_windows_$goArch.syso"
if ($LASTEXITCODE -ne 0) { throw "couldn't add the program icon" }
$env:GOOS = "windows"
$env:GOARCH = $goArch
& go build -C $root -trimpath -ldflags "-s -w -H windowsgui" -o $binary .
$buildResult = $LASTEXITCODE
Remove-Item Env:GOOS, Env:GOARCH
if ($buildResult -ne 0) { throw "go build failed" }

# The installer's license page needs RTF, so it is made from LICENSE each time.
$license = (Get-Content (Join-Path $root "LICENSE") -Raw) -replace '\{', '\{' -replace '\}', '\}'
$license = $license -replace "`r?`n", '\par '
Set-Content -Path (Join-Path $msiFolder "license.rtf") -Encoding Ascii `
    -Value "{\rtf1\ansi\deff0{\fonttbl{\f0 Segoe UI;}}\f0\fs18 $license}"

$msi = Join-Path $root "dist\Switaculous-$Version-$Arch.msi"
# WiX looks for the files the .wxs names in the bind path, so it points at this folder.
& $Wix build (Join-Path $PSScriptRoot "switaculous.wxs") -d "Version=$Version" -d "BinaryPath=$binary" `
    -bindpath $PSScriptRoot -bindpath $msiFolder -ext WixToolset.UI.wixext/5.0.2 -arch $Arch -o $msi
if ($LASTEXITCODE -ne 0) { throw "wix build failed" }
Write-Host "Wrote $msi"
