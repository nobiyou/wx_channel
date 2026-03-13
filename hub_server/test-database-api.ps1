# 数据库管理 API 测试脚本

Write-Host "=== 数据库管理 API 测试 ===" -ForegroundColor Cyan
Write-Host ""

# 配置
$baseUrl = "http://localhost:8080"
$token = Read-Host "请输入管理员 Token"

if ([string]::IsNullOrWhiteSpace($token)) {
    Write-Host "错误: Token 不能为空" -ForegroundColor Red
    exit 1
}

$headers = @{
    "Authorization" = "Bearer $token"
    "Content-Type" = "application/json"
}

# 测试 1: 获取数据库统计
Write-Host "测试 1: 获取数据库统计" -ForegroundColor Yellow
Write-Host "GET $baseUrl/api/admin/database/stats"
try {
    $response = Invoke-RestMethod -Uri "$baseUrl/api/admin/database/stats" -Method Get -Headers $headers
    Write-Host "✓ 成功" -ForegroundColor Green
    Write-Host "数据库大小: $($response.data.size_mb) MB" -ForegroundColor Cyan
    Write-Host "总记录数: $($response.data.total_records)" -ForegroundColor Cyan
    Write-Host "数据表数量: $($response.data.tables.Count)" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "数据表详情:" -ForegroundColor Cyan
    foreach ($table in $response.data.tables) {
        Write-Host "  - $($table.table_name): $($table.record_count) 条记录" -ForegroundColor White
        Write-Host "    最早: $($table.oldest_record), 最新: $($table.newest_record)" -ForegroundColor Gray
    }
} catch {
    Write-Host "✗ 失败: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# 测试 2: 优化数据库（可选）
Write-Host "测试 2: 优化数据库（可选）" -ForegroundColor Yellow
$optimize = Read-Host "是否执行优化？(y/n)"
if ($optimize -eq "y") {
    Write-Host "POST $baseUrl/api/admin/database/optimize"
    try {
        $response = Invoke-RestMethod -Uri "$baseUrl/api/admin/database/optimize" -Method Post -Headers $headers
        Write-Host "✓ 成功" -ForegroundColor Green
        Write-Host "优化结果:" -ForegroundColor Cyan
        foreach ($result in $response.data) {
            $status = if ($result.success) { "✓" } else { "✗" }
            Write-Host "  $status $($result.operation): $($result.duration)" -ForegroundColor $(if ($result.success) { "Green" } else { "Red" })
        }
    } catch {
        Write-Host "✗ 失败: $($_.Exception.Message)" -ForegroundColor Red
    }
} else {
    Write-Host "跳过优化测试" -ForegroundColor Gray
}
Write-Host ""

# 测试 3: 归档数据（可选）
Write-Host "测试 3: 归档旧数据（可选）" -ForegroundColor Yellow
$archive = Read-Host "是否执行归档？(y/n) [警告: 会删除数据]"
if ($archive -eq "y") {
    $browseMonths = Read-Host "浏览记录保留月数 (默认: 6)"
    $downloadYears = Read-Host "下载记录保留年数 (默认: 1)"
    $historyMonths = Read-Host "同步历史保留月数 (默认: 3)"
    
    if ([string]::IsNullOrWhiteSpace($browseMonths)) { $browseMonths = 6 }
    if ([string]::IsNullOrWhiteSpace($downloadYears)) { $downloadYears = 1 }
    if ([string]::IsNullOrWhiteSpace($historyMonths)) { $historyMonths = 3 }
    
    $body = @{
        browse_months = [int]$browseMonths
        download_years = [int]$downloadYears
        history_months = [int]$historyMonths
    } | ConvertTo-Json
    
    Write-Host "POST $baseUrl/api/admin/database/archive"
    Write-Host "配置: 浏览 $browseMonths 月, 下载 $downloadYears 年, 历史 $historyMonths 月" -ForegroundColor Gray
    
    $confirm = Read-Host "确认执行归档？(yes/no)"
    if ($confirm -eq "yes") {
        try {
            $response = Invoke-RestMethod -Uri "$baseUrl/api/admin/database/archive" -Method Post -Headers $headers -Body $body
            Write-Host "✓ 成功" -ForegroundColor Green
            Write-Host "删除统计:" -ForegroundColor Cyan
            Write-Host "  - 浏览记录: $($response.data.deleted_browse) 条" -ForegroundColor White
            Write-Host "  - 下载记录: $($response.data.deleted_download) 条" -ForegroundColor White
            Write-Host "  - 同步历史: $($response.data.deleted_history) 条" -ForegroundColor White
            Write-Host "  - 总计: $($response.data.total_deleted) 条" -ForegroundColor Yellow
        } catch {
            Write-Host "✗ 失败: $($_.Exception.Message)" -ForegroundColor Red
        }
    } else {
        Write-Host "已取消归档" -ForegroundColor Gray
    }
} else {
    Write-Host "跳过归档测试" -ForegroundColor Gray
}
Write-Host ""

Write-Host "=== 测试完成 ===" -ForegroundColor Cyan
