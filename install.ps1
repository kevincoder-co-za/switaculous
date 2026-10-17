# Switaculous - Swift & SwiftUI Learning Editor
#
# Builds Switaculous on Windows and installs it like any other app:
#   1. installs Go if it is missing (just for you, no administrator rights needed)
#   2. builds Switaculous and its Windows installer (MSI)
#   3. runs the installer, which adds Switaculous to the Start menu and can pin it to the taskbar
#   4. starts Switaculous
#
# Run it in the Switaculous folder:
#   powershell -ExecutionPolicy Bypass -File install.ps1

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Version = '1.0.0'
$MinimumGoMinorVersion = 24
$WixVersion = '5.0.2'
$BuildToolsFolder = Join-Path $env:LOCALAPPDATA 'Switaculous\build-tools'
# The copies that come with Windows, not ones from Git or MSYS that may be earlier on the PATH.
$Curl = Join-Path $env:SystemRoot 'System32\curl.exe'
$Tar = Join-Path $env:SystemRoot 'System32\tar.exe'

function Confirm-Step([string]$Question) {
    $answer = Read-Host "$Question [Y/n]"
    return ($answer -eq '' -or $answer -match '^[Yy]')
}

# Windows PowerShell turns anything a program writes to stderr into an error when stderr
# is redirected, so programs whose messages we want to hide are run through this.
function Invoke-Quietly([scriptblock]$Command) {
    $previous = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { & $Command 2>$null } finally { $ErrorActionPreference = $previous }
}

function Write-Step([string]$Message) {
    Write-Host ''
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Get-GoMinorVersion {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { return -1 }
    $goVersion = & go env GOVERSION
    if ($goVersion -match '^go1\.(\d+)') { return [int]$Matches[1] }
    return -1
}

function Get-CpuArchitecture {
    if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { return 'arm64' }
    return 'amd64'
}

# Go's zip needs no administrator rights: it goes to %LOCALAPPDATA%\Programs\go and onto
# your own PATH, the same place the Go installer would put it for a single user.
function Install-Go {
    $latest = & $Curl -fsSL 'https://go.dev/VERSION?m=text' | Select-Object -First 1
    if ($LASTEXITCODE -ne 0 -or -not $latest) { throw "Couldn't reach go.dev. Install Go from https://go.dev/doc/install and run this script again." }
    $latest = $latest.Trim()
    $zipName = "$latest.windows-$(Get-CpuArchitecture).zip"
    $zipPath = Join-Path $env:TEMP $zipName
    Write-Step "Downloading $latest"
    & $Curl -fL --progress-bar -o $zipPath "https://go.dev/dl/$zipName"
    if ($LASTEXITCODE -ne 0) { throw "Couldn't download Go. Install it from https://go.dev/dl/ and run this script again." }

    $programsFolder = Join-Path $env:LOCALAPPDATA 'Programs'
    $goFolder = Join-Path $programsFolder 'go'
    if (Test-Path $goFolder) { Remove-Item $goFolder -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $programsFolder | Out-Null
    & $Tar -xf $zipPath -C $programsFolder
    if ($LASTEXITCODE -ne 0) { throw "Couldn't unpack $zipName." }
    Remove-Item $zipPath

    # PATH is read and written unexpanded so entries like %USERPROFILE%\... keep working as they are.
    $goBin = Join-Path $goFolder 'bin'
    $environment = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    $userPath = $environment.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
    if (($userPath -split ';') -notcontains $goBin) {
        $environment.SetValue('Path', "$goBin;$userPath", 'ExpandString')
    }
    $environment.Close()
    $env:Path = "$goBin;$env:Path"
    Write-Host "Installed $latest in $goFolder"
}

function Find-DotNetWithSdk {
    $candidates = @(
        (Join-Path $BuildToolsFolder 'dotnet\dotnet.exe'),
        (Get-Command dotnet -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source)
    )
    foreach ($dotnet in $candidates) {
        if ($dotnet -and (Test-Path $dotnet)) {
            $sdks = Invoke-Quietly { & $dotnet --list-sdks }
            if ($sdks | Where-Object { $_ -match '^([6-9]|\d\d)\.' }) { return $dotnet }
        }
    }
    return $null
}

# Anything this function prints goes to the screen with Out-Host, because PowerShell would
# otherwise return it along with the path to wix.exe.
# The MSI is made with the WiX toolset, which runs on .NET. Both are installed just for
# this script in %LOCALAPPDATA%\Switaculous\build-tools and nothing else on the computer changes.
function Get-WixTool {
    $wix = Join-Path $BuildToolsFolder 'wix\wix.exe'
    $dotnet = Find-DotNetWithSdk
    if (-not (Test-Path $wix)) {
        if (-not $dotnet) {
            Write-Host 'Building the Windows installer needs the WiX toolset, which runs on the .NET SDK.'
            Write-Host "Both are installed only for this script, in $BuildToolsFolder (about 250 MB)."
            if (-not (Confirm-Step 'Download them now?')) { return $null }
            Write-Step 'Downloading the .NET SDK'
            $installer = Join-Path $env:TEMP 'dotnet-install.ps1'
            & $Curl -fsSL -o $installer 'https://dot.net/v1/dotnet-install.ps1'
            if ($LASTEXITCODE -ne 0) { throw "Couldn't download the .NET installer from dot.net." }
            & powershell -NoProfile -ExecutionPolicy Bypass -File $installer -Channel 8.0 -InstallDir (Join-Path $BuildToolsFolder 'dotnet') | Out-Host
            if ($LASTEXITCODE -ne 0) { throw "Couldn't install the .NET SDK." }
            $dotnet = Join-Path $BuildToolsFolder 'dotnet\dotnet.exe'
        }
        Write-Step "Installing WiX $WixVersion"
        & $dotnet tool install wix --version $WixVersion --tool-path (Join-Path $BuildToolsFolder 'wix') | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "Couldn't install WiX." }
    }
    # WiX looks for .NET in DOTNET_ROOT, and may run on a newer .NET than it was built for.
    if ($dotnet) { $env:DOTNET_ROOT = Split-Path $dotnet }
    $env:DOTNET_ROLL_FORWARD = 'Major'
    Invoke-Quietly { & $wix extension add -g "WixToolset.UI.wixext/$WixVersion" } | Out-Null
    return $wix
}

function Start-BuiltCopy([string]$SourceFolder) {
    $exe = Join-Path $SourceFolder 'dist\switaculous.exe'
    $goArch = Get-CpuArchitecture
    Write-Step 'Building switaculous.exe'
    & go -C $SourceFolder tool rsrc -ico packaging\windows\switaculous.ico -arch $goArch -o "rsrc_windows_$goArch.syso"
    & go build -C $SourceFolder -trimpath -ldflags '-s -w' -o $exe .
    if ($LASTEXITCODE -ne 0) { throw 'go build failed.' }
    Write-Step 'Starting Switaculous'
    Write-Host "You can start it again any time with: $exe"
    Start-Process -FilePath $exe
}

$sourceFolder = $PSScriptRoot
if (-not (Test-Path (Join-Path $sourceFolder 'go.mod'))) { throw "Run this script from the Switaculous folder it came in." }

Write-Host 'Switaculous - Swift & SwiftUI Learning Editor' -ForegroundColor Magenta

if ((Get-GoMinorVersion) -lt $MinimumGoMinorVersion) {
    Write-Host "Switaculous is built with Go 1.$MinimumGoMinorVersion or newer, which isn't installed yet."
    if (-not (Confirm-Step 'Download and install Go now?')) {
        Write-Host 'Install Go from https://go.dev/doc/install and run this script again.'
        exit 1
    }
    Install-Go
}
Write-Host "Using $(& go env GOVERSION)"

$wix = Get-WixTool
if (-not $wix) {
    Start-BuiltCopy $sourceFolder
    exit 0
}

$msiArch = if ((Get-CpuArchitecture) -eq 'arm64') { 'arm64' } else { 'x64' }
Write-Step 'Building the installer'
& (Join-Path $sourceFolder 'packaging\windows\build-msi.ps1') -Version $Version -Arch $msiArch -Wix $wix
$msi = Join-Path $sourceFolder "dist\Switaculous-$Version-$msiArch.msi"

if (-not (Confirm-Step 'Install Switaculous now? It will be added to your Start menu')) {
    Write-Host "The installer is ready whenever you want it: $msi"
    Start-BuiltCopy $sourceFolder
    exit 0
}

# The installer shows the usual license and finish pages. Its finish page offers to pin
# Switaculous to the taskbar (ticked by default) and then starts it.
# Waits for msiexec itself only: -Wait would also wait for Switaculous, which the installer starts.
$installer = Start-Process msiexec.exe -ArgumentList @('/i', "`"$msi`"") -PassThru
$null = $installer.Handle
$installer.WaitForExit()
switch ($installer.ExitCode) {
    0 { Write-Host 'Switaculous is installed. Find it in the Start menu.' -ForegroundColor Green }
    1602 { Write-Host 'Installation was cancelled.'; Start-BuiltCopy $sourceFolder }
    default { throw "The installer stopped with code $($installer.ExitCode)." }
}
