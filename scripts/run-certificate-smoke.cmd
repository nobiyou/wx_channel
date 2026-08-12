@echo off
setlocal
title WeChat Comment POC - Certificate Smoke Only
set "POC_WORK=%LOCALAPPDATA%\wechat-channel-comment-poc"
cd /d "%POC_WORK%\source"
".poc-build\wx_channel_poc.exe" cert-smoke --ack-isolated-vm
set "POC_EXIT=%ERRORLEVEL%"
echo Exit code: %POC_EXIT%
pause
exit /b %POC_EXIT%
