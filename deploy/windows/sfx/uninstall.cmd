@echo off
rem REQ-321 uninstall shim (logic lives in uninstall-core.ps1, parsed fully by PowerShell
rem before execution, so removing this directory mid-run is safe there)
rem NOTE: cd away from TARGET first — a cmd whose CWD sits inside the install dir would
rem block the finisher's directory removal (CWD lock)
chcp 65001 >nul
set "TARGET=%~dp0"
set "TARGET=%TARGET:/=\%"
if "%TARGET:~-1%"=="\" set "TARGET=%TARGET:~0,-1%"
cd /d "%TEMP%"
powershell -NoProfile -ExecutionPolicy Bypass -File "%TARGET%\uninstall-core.ps1" -Target "%TARGET%"
