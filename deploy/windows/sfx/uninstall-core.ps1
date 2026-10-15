# REQ-321 卸载核心（PowerShell：整本解析后执行，删自身目录安全）。
# 语义：停服 → 快捷方式/注册项移除 → 删安装目录。
# 数据根 %LOCALAPPDATA%\multiagent-lab 独立于程序目录（REQ-321 定案），卸载不触碰——天然保留。
param(
    [Parameter(Mandatory = $true)][string]$Target
)
$ErrorActionPreference = "Continue"
$log = Join-Path $env:TEMP "mlab-uninstall.log"
$keep = Join-Path $env:TEMP "MultiAgentLab-data-keep"
function Log($msg) { Add-Content -Path $log -Value ("[{0}] {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $msg) }
if ($Target.EndsWith("\")) { $Target = $Target.TrimEnd("\") }
Log "uninstall start TARGET=$Target"

Write-Host "[1/3] Stopping platform services..."
foreach ($img in @("MultiAgentLab.exe", "backend.exe", "ontologyd.exe", "runtimed.exe")) {
    Start-Process -FilePath "taskkill" -ArgumentList "/F", "/T", "/IM", $img -WindowStyle Hidden -Wait
}
Start-Sleep -Milliseconds 800

Write-Host "[2/3] Removing shortcuts and registry entry..."
$ps1 = Join-Path $Target "install-shortcuts.ps1"
if (Test-Path $ps1) {
    & powershell -NoProfile -ExecutionPolicy Bypass -File $ps1 -Remove -TargetDir $Target
} else {
    Log "install-shortcuts.ps1 missing, direct cleanup"
    $group = Join-Path ([Environment]::GetFolderPath("Programs")) "MultiAgentLab"
    if (Test-Path $group) { Remove-Item -Recurse -Force $group }
    $desk = Join-Path ([Environment]::GetFolderPath("Desktop")) "智能体构建平台.lnk"
    if (Test-Path $desk) { Remove-Item -Force $desk }
}
$regKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\MultiAgentLab"
if (Test-Path $regKey) { Remove-Item -Recurse -Force $regKey }

Write-Host "[3/3] Removing program files (data dir kept)..."
# 数据目录 = <安装目录>\data（data.location 由安装器写、launcher 读）；卸载时暂存到 %TEMP%，
# 由分离 finisher 删树后回迁——保证任何安装盘符选择下用户数据都不丢。
$hasData = Test-Path (Join-Path $Target "data")
if ($hasData) {
    if (Test-Path $keep) { Remove-Item -Recurse -Force $keep }
    robocopy (Join-Path $Target "data") $keep /E /MOVE /NFL /NDL /NJH /NJS /NP | Out-Null
    Log "data stash robocopy exit=$LASTEXITCODE"
}
# 删自身目录树必须由 %TEMP% 里的分离进程完成：本目录内有 cmd 仍在增量读取的 .cmd 与
# PowerShell 的执行现场，父进程直接 rd 会在删除瞬间中断读取方（实测整树删除后脚本静默死亡
# ——自删窗口期问题）。finisher 延迟 2 秒等父进程退出后收尾，重试 5 次容忍句柄释放延迟。
$fin = Join-Path $env:TEMP "MultiAgentLab-uninstall-finish.ps1"
@"
Start-Sleep -Seconds 2
foreach (`$i in 1..5) {
    Remove-Item -Recurse -Force '$Target' -ErrorAction SilentlyContinue
    if (-not (Test-Path '$Target')) { break }
    Start-Sleep -Seconds 2
}
New-Item -ItemType Directory -Force -Path (Join-Path '$Target' 'data') | Out-Null
if ('$hasData' -eq 'True' -and (Test-Path '$keep')) {
    robocopy '$keep' (Join-Path '$Target' 'data') /E /MOVE /NFL /NDL /NJH /NJS /NP | Out-Null
    Remove-Item -Recurse -Force '$keep' -ErrorAction SilentlyContinue
}
Add-Content -Path '$log' -Value ("[{0}] finisher done TARGET=$Target" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"))
"@ | Set-Content -Path $fin -Encoding UTF8
Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $fin -WindowStyle Hidden
Log "finisher spawned"
Write-Host "Uninstall complete. Data kept at: $env:LOCALAPPDATA\multiagent-lab (delete manually to purge)."
