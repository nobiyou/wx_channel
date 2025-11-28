# 日志清理工具
# 使用方法: .\scripts\clean_logs.ps1 [选项]

param(
    [string]$Action = "info",  # info, backup, clean, archive
    [int]$KeepDays = 7
)

$LogDir = "logs"
$BackupDir = "logs\backup"

function Show-Info {
    Write-Host "=== 日志文件信息 ===" -ForegroundColor Cyan
    
    if (-not (Test-Path $LogDir)) {
        Write-Host "日志目录不存在" -ForegroundColor Red
        return
    }
    
    $logFiles = Get-ChildItem -Path $LogDir -Filter "*.log" | Sort-Object LastWriteTime -Descending
    
    if ($logFiles.Count -eq 0) {
        Write-Host "没有找到日志文件" -ForegroundColor Yellow
        return
    }
    
    $totalSize = ($logFiles | Measure-Object -Property Length -Sum).Sum
    $totalSizeMB = [math]::Round($totalSize / 1MB, 2)
    
    Write-Host "日志文件数量: $($logFiles.Count)" -ForegroundColor Green
    Write-Host "总大小: $totalSizeMB MB" -ForegroundColor Green
    Write-Host ""
    
    Write-Host "文件列表:" -ForegroundColor Yellow
    foreach ($file in $logFiles) {
        $sizeMB = [math]::Round($file.Length / 1MB, 2)
        $age = (Get-Date) - $file.LastWriteTime
        Write-Host "  $($file.Name)" -ForegroundColor White
        Write-Host "    大小: $sizeMB MB | 最后修改: $($file.LastWriteTime) | 天数: $([math]::Floor($age.TotalDays))" -ForegroundColor Gray
    }
}

function Backup-Logs {
    Write-Host "=== 备份日志文件 ===" -ForegroundColor Cyan
    
    if (-not (Test-Path $LogDir)) {
        Write-Host "日志目录不存在" -ForegroundColor Red
        return
    }
    
    # 创建备份目录
    if (-not (Test-Path $BackupDir)) {
        New-Item -ItemType Directory -Path $BackupDir | Out-Null
        Write-Host "创建备份目录: $BackupDir" -ForegroundColor Green
    }
    
    $timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
    $backupFile = Join-Path $BackupDir "wx_channel_backup_$timestamp.zip"
    
    # 压缩日志文件
    $logFiles = Get-ChildItem -Path $LogDir -Filter "*.log"
    
    if ($logFiles.Count -eq 0) {
        Write-Host "没有找到需要备份的日志文件" -ForegroundColor Yellow
        return
    }
    
    Write-Host "正在备份 $($logFiles.Count) 个日志文件..." -ForegroundColor Yellow
    
    Compress-Archive -Path "$LogDir\*.log" -DestinationPath $backupFile -Force
    
    $backupSize = [math]::Round((Get-Item $backupFile).Length / 1MB, 2)
    Write-Host "备份完成: $backupFile ($backupSize MB)" -ForegroundColor Green
}

function Clean-OldLogs {
    param([int]$Days)
    
    Write-Host "=== 清理旧日志文件 ===" -ForegroundColor Cyan
    Write-Host "保留最近 $Days 天的日志" -ForegroundColor Yellow
    
    if (-not (Test-Path $LogDir)) {
        Write-Host "日志目录不存在" -ForegroundColor Red
        return
    }
    
    $cutoffDate = (Get-Date).AddDays(-$Days)
    $logFiles = Get-ChildItem -Path $LogDir -Filter "*.log" | Where-Object { $_.LastWriteTime -lt $cutoffDate }
    
    if ($logFiles.Count -eq 0) {
        Write-Host "没有需要清理的旧日志文件" -ForegroundColor Green
        return
    }
    
    Write-Host "找到 $($logFiles.Count) 个旧日志文件:" -ForegroundColor Yellow
    foreach ($file in $logFiles) {
        $age = (Get-Date) - $file.LastWriteTime
        Write-Host "  $($file.Name) (已存在 $([math]::Floor($age.TotalDays)) 天)" -ForegroundColor Gray
    }
    
    $confirm = Read-Host "确认删除这些文件？(y/n)"
    if ($confirm -eq 'y' -or $confirm -eq 'Y') {
        foreach ($file in $logFiles) {
            Remove-Item $file.FullName -Force
            Write-Host "  已删除: $($file.Name)" -ForegroundColor Green
        }
        Write-Host "清理完成！" -ForegroundColor Green
    } else {
        Write-Host "已取消清理操作" -ForegroundColor Yellow
    }
}

function Archive-Logs {
    Write-Host "=== 归档日志文件 ===" -ForegroundColor Cyan
    Write-Host "将先备份，然后清理旧日志" -ForegroundColor Yellow
    Write-Host ""
    
    # 先备份
    Backup-Logs
    Write-Host ""
    
    # 再清理
    Clean-OldLogs -Days $KeepDays
}

# 主逻辑
switch ($Action) {
    "info" {
        Show-Info
    }
    "backup" {
        Backup-Logs
    }
    "clean" {
        Clean-OldLogs -Days $KeepDays
    }
    "archive" {
        Archive-Logs
    }
    default {
        Write-Host "使用方法:" -ForegroundColor Yellow
        Write-Host "  查看信息: .\scripts\clean_logs.ps1 -Action info"
        Write-Host "  备份日志: .\scripts\clean_logs.ps1 -Action backup"
        Write-Host "  清理旧日志: .\scripts\clean_logs.ps1 -Action clean -KeepDays 7"
        Write-Host "  归档（备份+清理）: .\scripts\clean_logs.ps1 -Action archive -KeepDays 7"
    }
}
