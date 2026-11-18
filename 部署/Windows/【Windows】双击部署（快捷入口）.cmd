@echo off
setlocal DisableDelayedExpansion
where pwsh >nul 2>nul || (echo PowerShell 7.4+ required & exit /b 2)
set "shortcut_target="
set "shortcut_count=0"
for /d %%D in ("%~dp0Deployment\*.ps1") do (
  if exist "%%~fD\%%~nxD" (
    set /a shortcut_count+=1 >nul
    set "shortcut_target=%%~fD\%%~nxD"
  )
)
if not "%shortcut_count%"=="1" goto shortcut_missing
pwsh -NoLogo -NoProfile -File "%shortcut_target%" %*
set "shortcut_result=%errorlevel%"
if not "%shortcut_result%"=="0" pause
endlocal & exit /b %shortcut_result%
:shortcut_missing
>&2 echo Expected exactly one matching PowerShell implementation package.
endlocal & exit /b 2
