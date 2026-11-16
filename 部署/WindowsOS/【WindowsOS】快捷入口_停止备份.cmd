@echo off
where pwsh >nul 2>nul || (echo PowerShell 7.4+ required & exit /b 2)
if "%~1"=="" (pwsh -NoLogo -NoProfile -File "%~dp0停止备份.ps1\停止备份.ps1" stop) else (pwsh -NoLogo -NoProfile -File "%~dp0停止备份.ps1\停止备份.ps1" %*)
exit /b %errorlevel%
