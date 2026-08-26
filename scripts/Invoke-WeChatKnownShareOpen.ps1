[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ShareUrl
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not ('TrendRadar.WeChatChannelAutomation' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;

namespace TrendRadar {
    public static class WeChatChannelAutomation {
        public const int SW_RESTORE = 9;
        public const uint WM_KEYDOWN = 0x0100;
        public const uint WM_KEYUP = 0x0101;
        public const uint VK_RETURN = 0x0D;
        public const uint MOUSEEVENTF_LEFTDOWN = 0x0002;
        public const uint MOUSEEVENTF_LEFTUP = 0x0004;

        public delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);
        [StructLayout(LayoutKind.Sequential)]
        public struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
        [StructLayout(LayoutKind.Sequential)]
        public struct POINT { public int X; public int Y; }

        [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc callback, IntPtr lParam);
        [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
        [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr hWnd);
        [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);
        [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);
        [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
        [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
        [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int command);
        [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
        [DllImport("user32.dll")] public static extern bool BringWindowToTop(IntPtr hWnd);
        [DllImport("user32.dll")] public static extern uint GetCurrentThreadId();
        [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint idAttach, uint idAttachTo, bool attach);
        [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
        [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
        [DllImport("user32.dll")] public static extern bool GetCursorPos(out POINT point);
        [DllImport("user32.dll")] public static extern void mouse_event(uint flags, uint dx, uint dy, uint data, UIntPtr extra);
        [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr hWnd, uint message, IntPtr wParam, IntPtr lParam);

        public static bool ActivateWindow(IntPtr hWnd, int command) {
            IntPtr foreground = GetForegroundWindow();
            uint foregroundProcessId = 0;
            uint targetProcessId = 0;
            uint foregroundThread = foreground == IntPtr.Zero ? 0 : GetWindowThreadProcessId(foreground, out foregroundProcessId);
            uint targetThread = GetWindowThreadProcessId(hWnd, out targetProcessId);
            bool attached = foregroundThread != 0 && targetThread != 0 && foregroundThread != targetThread && AttachThreadInput(foregroundThread, targetThread, true);
            try {
                ShowWindow(hWnd, command);
                BringWindowToTop(hWnd);
                return SetForegroundWindow(hWnd);
            } finally {
                if (attached) AttachThreadInput(foregroundThread, targetThread, false);
            }
        }
    }
}
'@ -ErrorAction Stop
}

Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
[void][TrendRadar.WeChatChannelAutomation]::SetProcessDPIAware()

function Get-TopLevelWindowsByProcessName {
    param([Parameter(Mandatory = $true)][string]$ProcessName)
    $windows = [System.Collections.Generic.List[object]]::new()
    [void][TrendRadar.WeChatChannelAutomation]::EnumWindows({
        param($handle, $unused)
        if (-not [TrendRadar.WeChatChannelAutomation]::IsWindowVisible($handle)) { return $true }
        $titleBuilder = [Text.StringBuilder]::new(256)
        [void][TrendRadar.WeChatChannelAutomation]::GetWindowText($handle, $titleBuilder, $titleBuilder.Capacity)
        $title = $titleBuilder.ToString()
        [uint32]$processId = 0
        [void][TrendRadar.WeChatChannelAutomation]::GetWindowThreadProcessId($handle, [ref]$processId)
        $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
        if ($null -eq $process -or $process.ProcessName -cne $ProcessName -or [string]::IsNullOrWhiteSpace($title)) { return $true }
        $rect = [TrendRadar.WeChatChannelAutomation+RECT]::new()
        [void][TrendRadar.WeChatChannelAutomation]::GetWindowRect($handle, [ref]$rect)
        $windows.Add([pscustomobject]@{
            Handle = $handle
            HandleText = ('0x{0:X}' -f $handle.ToInt64())
            Title = $title
            ProcessName = $process.ProcessName
            Minimized = [TrendRadar.WeChatChannelAutomation]::IsIconic($handle)
            Left = $rect.Left
            Top = $rect.Top
            Width = $rect.Right - $rect.Left
            Height = $rect.Bottom - $rect.Top
        })
        return $true
    }, [IntPtr]::Zero)
    return @($windows)
}

function Get-GrayTemplateScore {
    param([Drawing.Bitmap]$Bitmap, [Drawing.Bitmap]$Template, [int]$CenterX, [int]$CenterY)
    $left = $CenterX - [int]($Template.Width / 2)
    $top = $CenterY - [int]($Template.Height / 2)
    if ($left -lt 0 -or $top -lt 0 -or ($left + $Template.Width) -gt $Bitmap.Width -or ($top + $Template.Height) -gt $Bitmap.Height) { return 0.0 }
    $sum = 0.0
    $count = 0
    for ($y = 0; $y -lt $Template.Height; $y += 2) {
        for ($x = 0; $x -lt $Template.Width; $x += 2) {
            $a = $Bitmap.GetPixel($left + $x, $top + $y)
            $b = $Template.GetPixel($x, $y)
            $grayA = 0.299 * $a.R + 0.587 * $a.G + 0.114 * $a.B
            $grayB = 0.299 * $b.R + 0.587 * $b.G + 0.114 * $b.B
            $sum += [Math]::Abs($grayA - $grayB) / 255.0
            $count++
        }
    }
    if ($count -eq 0) { return 0.0 }
    return [Math]::Round(1.0 - ($sum / $count), 4)
}

function Read-EntryTemplate {
    $templatePath = Join-Path $PSScriptRoot 'wechat-channel-entry-template.b64'
    if (-not [IO.File]::Exists($templatePath)) { throw 'wechat_entry_template_missing' }
    try {
        $bytes = [Convert]::FromBase64String(([IO.File]::ReadAllText($templatePath)).Trim())
        $stream = [IO.MemoryStream]::new($bytes)
        try {
            $loaded = [Drawing.Bitmap]::new($stream)
            try { return [Drawing.Bitmap]::new($loaded) }
            finally { $loaded.Dispose() }
        } finally { $stream.Dispose() }
    } catch { throw 'wechat_entry_template_invalid' }
}

function Test-ShareUrl {
    param([string]$Value)
    try { $uri = [Uri]$Value } catch { return $false }
    if ($uri.Scheme -cne 'https' -or $uri.Host -notin @('weixin.qq.com', 'channels.weixin.qq.com')) { return $false }
    return $uri.AbsolutePath -match '^/sph(?:/|$)|^/finder-preview/pages/sph(?:/|$)'
}

if (-not (Test-ShareUrl -Value $ShareUrl)) { throw 'wechat_share_url_invalid' }

$mainWindows = @(Get-TopLevelWindowsByProcessName -ProcessName 'Weixin')
if ($mainWindows.Count -eq 0) { $mainWindows = @(Get-TopLevelWindowsByProcessName -ProcessName 'WeChat') }
if ($mainWindows.Count -eq 0) { throw 'wechat_window_not_found' }
if ($mainWindows.Count -ne 1) { throw 'wechat_window_ambiguous' }
$main = $mainWindows[0]

if (-not [TrendRadar.WeChatChannelAutomation]::ActivateWindow($main.Handle, [TrendRadar.WeChatChannelAutomation]::SW_RESTORE)) { throw 'wechat_window_activation_failed' }
Start-Sleep -Milliseconds 150
$rect = [TrendRadar.WeChatChannelAutomation+RECT]::new()
[void][TrendRadar.WeChatChannelAutomation]::GetWindowRect($main.Handle, [ref]$rect)
$main.Left = $rect.Left; $main.Top = $rect.Top; $main.Width = $rect.Right - $rect.Left; $main.Height = $rect.Bottom - $rect.Top
if ($main.Width -lt 500 -or $main.Height -lt 400) { throw 'wechat_window_invalid_size' }

$bitmap = [Drawing.Bitmap]::new($main.Width, $main.Height)
$template = $null
try {
    $graphics = [Drawing.Graphics]::FromImage($bitmap)
    $graphics.CopyFromScreen($main.Left, $main.Top, 0, 0, $bitmap.Size)
    $graphics.Dispose()
    $template = Read-EntryTemplate
    $matches = @(foreach ($xRatio in @(0.035, 0.042, 0.049)) {
        foreach ($yRatio in @(0.44, 0.455, 0.47, 0.485, 0.50)) {
            $cx = [int]($bitmap.Width * $xRatio); $cy = [int]($bitmap.Height * $yRatio)
            $templateScore = Get-GrayTemplateScore -Bitmap $bitmap -Template $template -CenterX $cx -CenterY $cy
            $geometryDistance = [Math]::Abs($xRatio - 0.042) + [Math]::Abs($yRatio - 0.47)
            $geometryScore = [Math]::Round([Math]::Max(0.0, 1.0 - ($geometryDistance / 0.08)), 4)
            [pscustomobject]@{ X = $cx; Y = $cy; TemplateScore = $templateScore; GeometryScore = $geometryScore; CombinedScore = [Math]::Round((0.6 * $templateScore) + (0.4 * $geometryScore), 4); GeometryPass = ($xRatio -ge 0.03 -and $xRatio -le 0.055 -and $yRatio -ge 0.43 -and $yRatio -le 0.51) }
        }
    })
    $ranked = @($matches | Sort-Object CombinedScore -Descending)
    $best = $ranked[0]
    $margin = if ($ranked.Count -gt 1) { $best.CombinedScore - $ranked[1].CombinedScore } else { 1.0 }
    Write-Verbose ("window={0} rect={1},{2} {3}x{4} best={5},{6} template={7} combined={8} margin={9}" -f $main.HandleText, $main.Left, $main.Top, $main.Width, $main.Height, $best.X, $best.Y, $best.TemplateScore, $best.CombinedScore, $margin)
    if (-not $best.GeometryPass -or $best.TemplateScore -lt 0.88 -or $margin -lt 0.02) { throw 'wechat_entry_template_mismatch' }

    $cursor = [TrendRadar.WeChatChannelAutomation+POINT]::new()
    [void][TrendRadar.WeChatChannelAutomation]::GetCursorPos([ref]$cursor)
    $absoluteX = $main.Left + $best.X; $absoluteY = $main.Top + $best.Y
    [void][TrendRadar.WeChatChannelAutomation]::SetCursorPos($absoluteX, $absoluteY)
    Start-Sleep -Milliseconds 100
    [TrendRadar.WeChatChannelAutomation]::mouse_event([TrendRadar.WeChatChannelAutomation]::MOUSEEVENTF_LEFTDOWN, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 60
    [TrendRadar.WeChatChannelAutomation]::mouse_event([TrendRadar.WeChatChannelAutomation]::MOUSEEVENTF_LEFTUP, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 250
    [void][TrendRadar.WeChatChannelAutomation]::SetCursorPos($cursor.X, $cursor.Y)
} finally {
    if ($null -ne $template) { $template.Dispose() }
    $bitmap.Dispose()
}

$webView = $null
for ($attempt = 0; $attempt -lt 30; $attempt++) {
    $webViews = @(Get-TopLevelWindowsByProcessName -ProcessName 'WeChatAppEx' | Where-Object { $_.Width -gt 300 -and $_.Height -gt 200 })
    if ($webViews.Count -eq 1) { $webView = $webViews[0]; break }
    if ($webViews.Count -gt 1) { throw 'wechat_webview_ambiguous' }
    Start-Sleep -Milliseconds 500
}
if ($null -eq $webView) { throw 'wechat_webview_not_ready' }
if (-not [TrendRadar.WeChatChannelAutomation]::ActivateWindow($webView.Handle, [TrendRadar.WeChatChannelAutomation]::SW_RESTORE)) { throw 'wechat_webview_activation_failed' }
Start-Sleep -Milliseconds 200

$root = [System.Windows.Automation.AutomationElement]::FromHandle($webView.Handle)
$editCondition = [System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Edit)
$edits = @($root.FindAll([System.Windows.Automation.TreeScope]::Descendants, $editCondition))
$matches = @(foreach ($edit in $edits) {
    $className = [string]$edit.Current.ClassName
    $automationId = [string]$edit.Current.AutomationId
    if ($className -ne 'OmniboxViewViews' -and $automationId -ne 'OmniboxViewViews') { continue }
    $pattern = $null
    if (-not $edit.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern, [ref]$pattern)) { continue }
    if ($edit.Current.IsEnabled -and -not $edit.Current.IsOffscreen -and -not $pattern.Current.IsReadOnly) { [pscustomobject]@{ Element = $edit; Pattern = $pattern } }
})
if ($matches.Count -ne 1) { throw 'wechat_share_address_bar_not_ready' }
$matches[0].Pattern.SetValue($ShareUrl)
if (-not [TrendRadar.WeChatChannelAutomation]::PostMessage($webView.Handle, [TrendRadar.WeChatChannelAutomation]::WM_KEYDOWN, [IntPtr][TrendRadar.WeChatChannelAutomation]::VK_RETURN, [IntPtr]::Zero)) { throw 'wechat_share_navigation_failed' }
[void][TrendRadar.WeChatChannelAutomation]::PostMessage($webView.Handle, [TrendRadar.WeChatChannelAutomation]::WM_KEYUP, [IntPtr][TrendRadar.WeChatChannelAutomation]::VK_RETURN, [IntPtr]::Zero)
'wechat_known_share_opened'
