@echo off
setlocal

set "SCRIPT_DIR=%~dp0"
set "ADMIN_SCRIPT=%SCRIPT_DIR%switch-insight-data-plane-admin.ps1"

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%ADMIN_SCRIPT%"
