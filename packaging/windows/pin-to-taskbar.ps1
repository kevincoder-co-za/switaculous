# Run by the installer when "Pin Switaculous to the taskbar" is ticked.
# Windows 10 still lets an installer use the taskbar's own "Pin to taskbar" command.
# Windows 11 only lets people pin apps themselves, so there we explain how.
$shortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'Switaculous.lnk'
$pinnedFolder = Join-Path $env:APPDATA 'Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar'
$verbKey = 'Software\Classes\*\shell\SwitaculousPinToTaskbar'

function Test-Pinned {
    $pinned = Get-ChildItem -LiteralPath $pinnedFolder -Filter '*.lnk' -ErrorAction SilentlyContinue
    return [bool]($pinned | Where-Object { $_.BaseName -eq 'Switaculous' })
}

if (Test-Pinned) { exit 0 }

try {
    $pinCommand = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\CommandStore\shell\Windows.taskbarpin' -ErrorAction Stop
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($verbKey)
    $key.SetValue('ExplorerCommandHandler', $pinCommand.ExplorerCommandHandler)
    $key.Close()
    $shell = New-Object -ComObject Shell.Application
    $item = $shell.Namespace((Split-Path $shortcut)).ParseName((Split-Path $shortcut -Leaf))
    $item.InvokeVerb('SwitaculousPinToTaskbar')
} catch {
} finally {
    [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($verbKey, $false)
}

Start-Sleep -Milliseconds 800
if (-not (Test-Pinned)) {
    Add-Type -AssemblyName System.Windows.Forms
    $message = "Windows doesn't let installers pin apps to the taskbar, so please pin Switaculous yourself:`n`n" +
        "Open Start, type Switaculous, right-click it and choose Pin to taskbar."
    [System.Windows.Forms.MessageBox]::Show($message, 'Switaculous', 'OK', 'Information', 'Button1', 'DefaultDesktopOnly') | Out-Null
}
