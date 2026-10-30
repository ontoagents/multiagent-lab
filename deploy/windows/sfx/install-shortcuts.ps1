# REQ-321 安装器辅助：快捷方式（开始菜单分组 + 桌面）与 HKCU 卸载注册项
# 用法：install-shortcuts.ps1 -Install [-TargetDir <dir>] ｜ install-shortcuts.ps1 -Remove [-TargetDir <dir>]
# TargetDir=实际安装目录（安装位置可自定义，REQ-321；缺省回退 %LOCALAPPDATA% 旧默认）
param([switch]$Install, [switch]$Remove, [string]$TargetDir)
$ErrorActionPreference = "Stop"

$target  = $TargetDir
if (-not $target) { $target = Join-Path $env:LOCALAPPDATA "Programs\OntoAgents" }
$progMenu = [Environment]::GetFolderPath("Programs")
$group   = Join-Path $progMenu "OntoAgents"
$desktop = [Environment]::GetFolderPath("Desktop")
$regKey  = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\OntoAgents"

if ($Remove) {
    if (Test-Path $group) { Remove-Item -Recurse -Force $group }
    $deskLnk = Join-Path $desktop "智能体构建平台.lnk"
    if (Test-Path $deskLnk) { Remove-Item -Force $deskLnk }
    if (Test-Path $regKey) { Remove-Item -Recurse -Force $regKey }
    exit 0
}

if (-not $Install) { throw "用法：install-shortcuts.ps1 -Install ｜ -Remove" }
if (-not (Test-Path (Join-Path $target "OntoAgents.exe"))) { throw "未找到 $target\OntoAgents.exe（请先复制程序文件）" }

New-Item -ItemType Directory -Force -Path $group | Out-Null
$ws = New-Object -ComObject WScript.Shell
function New-Link($path, $exe, $lnkArgs, $desc, $workdir) {
    $s = $ws.CreateShortcut($path)
    $s.TargetPath = $exe
    $s.Arguments = $lnkArgs
    $s.Description = $desc
    $s.WorkingDirectory = $workdir
    $s.Save()
}
New-Link (Join-Path $group "智能体构建平台.lnk")     "$target\OntoAgents.exe" ""      "启动智能体构建平台（自动拉起本地服务）" $target
New-Link (Join-Path $group "停止平台服务.lnk")       "$target\OntoAgents.exe" "-stop" "停止平台后台服务" $target
# 卸载快捷方式的起始目录刻意用 %TEMP%：卸载器会删安装目录，CWD 停在里面会让空目录无法移除
New-Link (Join-Path $group "卸载智能体构建平台.lnk") "$target\uninstall.cmd"     ""      "卸载（保留数据目录 data）" $env:TEMP
New-Link (Join-Path $desktop "智能体构建平台.lnk")   "$target\OntoAgents.exe" ""      "启动智能体构建平台" $target

# HKCU 卸载注册项（「应用与功能」可见）
New-Item -Force -Path $regKey | Out-Null
Set-ItemProperty $regKey -Name DisplayName     -Value "智能体构建平台"
Set-ItemProperty $regKey -Name DisplayVersion  -Value "1.0.0"
Set-ItemProperty $regKey -Name Publisher       -Value "ontoagents"
Set-ItemProperty $regKey -Name InstallLocation -Value $target
Set-ItemProperty $regKey -Name DisplayIcon     -Value (Join-Path $target "OntoAgents.exe")
Set-ItemProperty $regKey -Name UninstallString -Value ('"' + (Join-Path $target "uninstall.cmd") + '"')
Set-ItemProperty $regKey -Name NoModify        -Value 1 -Type DWord
Set-ItemProperty $regKey -Name NoRepair        -Value 1 -Type DWord
$bytes = (Get-ChildItem -Recurse -Force -File $target | Measure-Object Length -Sum).Sum
Set-ItemProperty $regKey -Name EstimatedSize   -Value ([int]($bytes / 1KB)) -Type DWord
exit 0
