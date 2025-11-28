# 日志分析工具 - 生成统计报告
# 使用方法: .\scripts\analyze_logs.ps1

$LogFile = "logs\wx_channel.log"

if (-not (Test-Path $LogFile)) {
    Write-Host "日志文件不存在: $LogFile" -ForegroundColor Red
    exit 1
}

Write-Host "================================" -ForegroundColor Cyan
Write-Host "   wx_channel 日志分析报告" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

# 基本统计
$content = Get-Content $LogFile
$totalLines = $content.Count
Write-Host "📊 基本统计" -ForegroundColor Green
Write-Host "  总日志条数: $totalLines"
Write-Host ""

# 系统事件
Write-Host "🖥️  系统事件" -ForegroundColor Green
$startCount = (Select-String -Path $LogFile -Pattern '系统启动').Count
$shutdownCount = (Select-String -Path $LogFile -Pattern '系统关闭').Count
Write-Host "  系统启动: $startCount 次"
Write-Host "  系统关闭: $shutdownCount 次"
Write-Host ""

# 页面访问
Write-Host "📄 页面访问" -ForegroundColor Green
$feedCount = (Select-String -Path $LogFile -Pattern 'Path=/web/pages/feed').Count
$homeCount = (Select-String -Path $LogFile -Pattern 'Path=/web/pages/home').Count
$profileCount = (Select-String -Path $LogFile -Pattern 'Path=/web/pages/profile').Count
$searchCount = (Select-String -Path $LogFile -Pattern 'Path=/web/pages/search').Count
Write-Host "  Feed页: $feedCount 次"
Write-Host "  Home页: $homeCount 次"
Write-Host "  Profile页: $profileCount 次"
Write-Host "  Search页: $searchCount 次"
Write-Host ""

# 下载统计
Write-Host "⬇️  下载统计" -ForegroundColor Green
$downloadCount = (Select-String -Path $LogFile -Pattern '下载记录').Count
$coverCount = (Select-String -Path $LogFile -Pattern '下载封面').Count
$formatCount = (Select-String -Path $LogFile -Pattern '格式下载').Count
Write-Host "  视频下载: $downloadCount 次"
Write-Host "  封面下载: $coverCount 次"
Write-Host "  格式下载: $formatCount 次"
Write-Host ""

# 数据采集
Write-Host "📝 数据采集" -ForegroundColor Green
$commentCount = (Select-String -Path $LogFile -Pattern '评论采集').Count
$csvAddCount = (Select-String -Path $LogFile -Pattern 'CSV操作.*成功.*添加记录').Count
$csvFailCount = (Select-String -Path $LogFile -Pattern 'CSV操作.*失败').Count
Write-Host "  评论采集: $commentCount 次"
Write-Host "  CSV添加成功: $csvAddCount 次"
Write-Host "  CSV添加失败: $csvFailCount 次（重复记录）"
Write-Host ""

# 错误统计
Write-Host "❌ 错误统计" -ForegroundColor Yellow
$errorCount = (Select-String -Path $LogFile -Pattern 'ERROR').Count
$allFailCount = (Select-String -Path $LogFile -Pattern '失败').Count
$csvFailCount2 = (Select-String -Path $LogFile -Pattern 'CSV操作.*失败').Count
$otherFailCount = $allFailCount - $csvFailCount2
Write-Host "  ERROR级别: $errorCount 次"
Write-Host "  其他失败: $otherFailCount 次"
Write-Host ""

# 最近活动
Write-Host "🕐 最近活动（最后10条）" -ForegroundColor Green
Get-Content $LogFile -Tail 10 | ForEach-Object {
    Write-Host "  $_"
}
Write-Host ""

Write-Host "================================" -ForegroundColor Cyan
Write-Host "分析完成！" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
