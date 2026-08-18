[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not ('TrendRadar.WeChatWindow' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

namespace TrendRadar {
    public static class WeChatWindow {
        public const uint WM_KEYDOWN = 0x0100;
        public const uint WM_KEYUP = 0x0101;
        public const uint VK_F5 = 0x74;

        [DllImport("user32.dll", SetLastError = true)]
        public static extern bool PostMessage(IntPtr hWnd, uint message, IntPtr wParam, IntPtr lParam);

        [DllImport("user32.dll")]
        public static extern IntPtr GetForegroundWindow();

        [DllImport("user32.dll")]
        public static extern bool IsWindowVisible(IntPtr hWnd);
    }
}
'@ -ErrorAction Stop
}

$processNames = @('Weixin', 'WeChat', 'WeChatAppEx')
$candidates = @(
    foreach ($name in $processNames) {
        foreach ($process in @(Get-Process -Name $name -ErrorAction SilentlyContinue)) {
            try {
                if ($process.MainWindowHandle -eq [IntPtr]::Zero) { continue }
                if (-not [TrendRadar.WeChatWindow]::IsWindowVisible($process.MainWindowHandle)) { continue }
                if ([string]::IsNullOrWhiteSpace([string]$process.MainWindowTitle)) { continue }
                [pscustomobject]@{ Handle = $process.MainWindowHandle; ProcessName = $process.ProcessName }
            } catch { }
        }
    }
)

if ($candidates.Count -eq 0) { throw 'wechat_window_not_found' }

$foreground = [TrendRadar.WeChatWindow]::GetForegroundWindow()
$foregroundCandidates = @($candidates | Where-Object { $_.Handle -eq $foreground })
if ($foregroundCandidates.Count -eq 1) {
    $selected = $foregroundCandidates[0]
} elseif (@($candidates | Where-Object { $_.ProcessName -eq 'Weixin' }).Count -eq 1) {
    # WeChatAppEx may expose a second titled child window; prefer the single
    # visible Weixin host process as the stable PC WeChat main window.
    $selected = @($candidates | Where-Object { $_.ProcessName -eq 'Weixin' })[0]
} elseif ($candidates.Count -eq 1) {
    $selected = $candidates[0]
} else {
    throw 'wechat_window_ambiguous'
}

if (-not [TrendRadar.WeChatWindow]::PostMessage($selected.Handle, [TrendRadar.WeChatWindow]::WM_KEYDOWN, [IntPtr][TrendRadar.WeChatWindow]::VK_F5, [IntPtr]::Zero)) {
    throw 'wechat_page_refresh_failed'
}
[void][TrendRadar.WeChatWindow]::PostMessage($selected.Handle, [TrendRadar.WeChatWindow]::WM_KEYUP, [IntPtr][TrendRadar.WeChatWindow]::VK_F5, [IntPtr]::Zero)
'wechat_page_refresh_sent'
