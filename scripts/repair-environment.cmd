@echo off
setlocal
set "SCRIPT_DIR=%~dp0"

echo Repairing wx_channel environment...
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%repair-environment.ps1" %*
set "EXIT_CODE=%ERRORLEVEL%"

echo.
if "%EXIT_CODE%"=="0" (
    echo Repair completed: no failures.
) else if "%EXIT_CODE%"=="1" (
    echo Repair completed: warnings were reported above.
) else (
    echo Repair failed: review the error and advice above.
)
echo Exit code: %EXIT_CODE%
pause
exit /b %EXIT_CODE%