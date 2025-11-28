# 日志查看工具
# 使用方法: .\scripts\view_logs.ps1 [选项]

param(
    [string]$Action = "tail",  # tail, search, stats, errors
    [string]$Pattern = "",
    [int]$Lines = 50
)

$LogFile = "logs\wx_channel.log"

if (-not (Test-Path $LogFile)) {
    Write-Host "日志文件不存在: $LogFile" -ForegroundColor Red
    exit 1
}

switch ($Action) {
    "tail" {
        Write-Host "=== 实时查看日志（最近 $Lines 行）===" -ForegroundColor Green
        Get-Content $LogFile -Tail $Lines -Wait
    }
    "search" {
        if ($Pattern -eq "") {
            Write-Host "请提供搜索关键词: -Pattern '关键词'" -ForegroundColor Red
            exit 1
        }
        Write-Host "=== 搜索日志: $Pattern ===" -ForegroundColor Green
        Select-String -Path $LogFile -Pattern $Pattern | ForEach-Object {
            Write-Host $_.Line
        }
    }
    "stats" {
        Write-Host "=== 日志统计 ===" -ForegroundColor Green
        $content = Get-Content $LogFile
        Write-Host "总行数: $($content.Count)"
        Write-Host "下载记录: $((Select-String -Path $LogFile -Pattern '下载记录').Count)"
        Write-Host "评论采集: $((Select-String -Path $LogFile -Pattern '评论采集').Count)"
        Write-Host "页面加载: $((Select-String -Path $LogFile -Pattern '页面加载').Count)"
        Write-Host "系统启动: $((Select-String -Path $LogFile -Pattern '系统启动').Count)"
        Write-Host "错误数量: $((Select-String -Path $LogFile -Pattern 'ERROR').Count)"
    }
    "errors" {
        Write-Host "=== 错误日志 ===" -ForegroundColor Red
        Select-String -Path $LogFile -Pattern "ERROR|失败" | ForEach-Object {
            Write-Host $_.Line -ForegroundColor Red
        }
    }
    default {
        Write-Host "使用方法:" -ForegroundColor Yellow
        Write-Host "  实时查看: .\scripts\view_logs.ps1 -Action tail"
        Write-Host "  搜索日志: .\scripts\view_logs.ps1 -Action search -Pattern '关键词'"
        Write-Host "  统计信息: .\scripts\view_logs.ps1 -Action stats"
        Write-Host "  查看错误: .\scripts\view_logs.ps1 -Action errors"
    }
}
