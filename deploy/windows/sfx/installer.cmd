@echo off
rem REQ-321 SFX installer main script (run by 7zSD.sfx from extraction dir %~dp0)
rem NOTE: keep first lines pure ASCII; chcp 65001 before any non-ASCII text; CRLF required (build.ps1 normalizes)
chcp 65001 >nul
setlocal
set "TARGET=%LOCALAPPDATA%\Programs\MultiAgentLab"
set "SRC=%~dp0"
echo [%date% %time%] installer start SRC="%SRC%" TARGET="%TARGET%" SILENT="%MLAB_SILENT%" >> "%TEMP%\mlab-install.log"

echo [1/4] Stopping running platform services (upgrade/reinstall)...
taskkill /F /T /IM MultiAgentLab.exe >nul 2>&1
taskkill /F /T /IM backend.exe >nul 2>&1
taskkill /F /T /IM ontologyd.exe >nul 2>&1
taskkill /F /T /IM runtimed.exe >nul 2>&1
ping -n 2 127.0.0.1 >nul

echo [2/4] Copying program files to %TARGET% ...
if not exist "%TARGET%" md "%TARGET%" >nul 2>&1
if errorlevel 1 goto :fail
robocopy "%SRC%payload\app" "%TARGET%" /E /NFL /NDL /NJH /NJS /NP >nul
echo [%date% %time%] robocopy exit=%errorlevel% >> "%TEMP%\mlab-install.log"
if errorlevel 8 goto :fail
copy /y "%SRC%uninstall.cmd" "%TARGET%\uninstall.cmd" >nul
copy /y "%SRC%uninstall-core.ps1" "%TARGET%\uninstall-core.ps1" >nul
copy /y "%SRC%install-shortcuts.ps1" "%TARGET%\install-shortcuts.ps1" >nul

echo [3/4] Creating shortcuts and uninstall registry entry...
powershell -NoProfile -ExecutionPolicy Bypass -File "%SRC%install-shortcuts.ps1" -Install
if errorlevel 1 goto :fail

echo [4/4] Done.
echo   Start: Start Menu / Desktop "智能体构建平台" (auto-starts local services)
echo   Data:  %TARGET%\data (kept on uninstall)
echo   Guide: %TARGET%\README.txt
echo [%date% %time%] installer OK >> "%TEMP%\mlab-install.log"
if /i "%MLAB_SILENT%"=="1" exit /b 0
choice /c YN /m "Launch now"
if errorlevel 2 exit /b 0
start "" "%TARGET%\MultiAgentLab.exe"
exit /b 0

:fail
echo [%date% %time%] installer FAIL >> "%TEMP%\mlab-install.log"
echo [ERROR] Install failed (target %TARGET%). Close platform windows and retry.
exit /b 1
