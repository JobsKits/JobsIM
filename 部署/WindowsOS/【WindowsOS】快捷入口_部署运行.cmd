@echo off
where pwsh >nul 2>nul || (echo PowerShell 7.4+ required & exit /b 2)
if "%~1"=="" (pwsh -NoLogo -NoProfile -File "%~dp0部署运行.ps1\部署运行.ps1" run) else (pwsh -NoLogo -NoProfile -File "%~dp0部署运行.ps1\部署运行.ps1" %*)
exit /b %errorlevel%
