@echo off
setlocal
set "SCRIPT_DIR=%~dp0"

echo Checking wx_channel environment...
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%check-environment.ps1" %*
set "EXIT_CODE=%ERRORLEVEL%"

echo.
if "%EXIT_CODE%"=="0" (
    echo Check completed: no failures.
) else if "%EXIT_CODE%"=="1" (
    echo Check completed: warnings were reported above.
) else (
    echo Check failed: review the error and advice above.
)
echo Exit code: %EXIT_CODE%
pause
exit /b %EXIT_CODE%