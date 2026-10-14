# REQ-321 Windows 桌面安装包构建脚本（PowerShell 5.1+）
# 用法：
#   powershell -ExecutionPolicy Bypass -File build.ps1                 # 全量：三服务+壳+前端+staging+setup.exe
#   powershell -ExecutionPolicy Bypass -File build.ps1 -SkipWeb        # 跳过前端构建（web/dist 未变时）
#   powershell -ExecutionPolicy Bypass -File build.ps1 -StageOnly      # 只组装 staging，不编译安装包
# 产物：deploy/windows/dist/MultiAgentLab_<版本>_setup.exe（per-user 免管理员安装）
# 依赖：Go 1.25+、Node 22+、ISCC（Inno Setup 6；env ISCC 或默认安装路径自动发现）

param(
    [switch]$SkipWeb,
    [switch]$StageOnly,
    [switch]$IncludeOxigraph   # REQ-321 定案：oxigraph 默认零捆绑（首启运行页一键安装），此开关显式预置
)

$ErrorActionPreference = "Stop"
$here     = Split-Path -Parent $MyInvocation.MyCommand.Path   # deploy/windows
$repo     = Split-Path -Parent (Split-Path -Parent $here)     # 仓库根
$build    = Join-Path $here "build"
$cache    = Join-Path $build "cache"
$staging  = Join-Path $build "staging"
$dist     = Join-Path $here "dist"
$version  = "1.0.0"

function Step($msg) { Write-Host "[build] $msg" -ForegroundColor Cyan }

# ---- 0. staging 重置（保留 cache）----
Step "重置 staging：$staging"
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
New-Item -ItemType Directory -Force -Path @(
    "$staging\bin", "$staging\web",
    "$staging\migrations\ontology-service", "$staging\migrations\runtime-manager",
    "$staging\tools\rdf-sidecar", $cache, $dist
) | Out-Null

# ---- 1. 三服务（现代码纯 Go；-s -w 缩体积）----
Step "编译三服务（windows/amd64）"
Push-Location "$repo\backend";           go build -trimpath -ldflags "-s -w" -o "$staging\bin\backend.exe" ./cmd/backend; Pop-Location
Push-Location "$repo\ontology-service";  go build -trimpath -ldflags "-s -w" -o "$staging\bin\ontologyd.exe" ./cmd/ontologyd; Pop-Location
Push-Location "$repo\runtime-manager";   go build -trimpath -ldflags "-s -w" -o "$staging\bin\runtimed.exe" ./cmd/runtimed; Pop-Location

# ---- 2. 客户端壳（独立 module，GOWORK=off 防 go.work 连坐）----
Step "编译客户端壳 MultiAgentLab.exe（WebView2）"
Push-Location "$here\launcher"
$env:GOWORK = "off"; go build -trimpath -ldflags "-s -w -H windowsgui" -o "$staging\MultiAgentLab.exe" .
Remove-Item Env:\GOWORK; Pop-Location

# ---- 3. 前端（tsc + vite → staging\web\dist）----
if ($SkipWeb -and (Test-Path "$repo\web\dist\index.html")) {
    Step "跳过前端构建（-SkipWeb，沿用现有 web/dist）"
} else {
    Step "构建前端（npm run build）"
    Push-Location "$repo\web"; npm install --no-audit --no-fund; npm run build; Pop-Location
}
Copy-Item -Recurse -Force "$repo\web\dist" "$staging\web\dist"

# ---- 4. oxigraph Windows 引擎（REQ-321 定案：默认零捆绑，首启运行页一键安装；-IncludeOxigraph 显式预置）----
$oxi = "$staging\bin\oxigraph.exe"
if ($IncludeOxigraph -and -not (Test-Path $oxi)) {
    $cached = "$cache\oxigraph.exe"
    if (-not (Test-Path $cached)) {
        Step "抓取 oxigraph v0.5.11 windows 资产"
        Invoke-WebRequest -UseBasicParsing `
            -Uri "https://github.com/oxigraph/oxigraph/releases/download/v0.5.11/oxigraph_v0.5.11_x86_64_windows_msvc.exe" `
            -OutFile $cached
    }
    Copy-Item $cached $oxi
} elseif (Test-Path $oxi) {
    Copy-Item $oxi "$cache\oxigraph.exe" -Force
    Remove-Item $oxi
    Step "oxigraph 不随包（REQ-321 零捆绑），已移出 staging（缓存保留）"
}

# ---- 5. 迁移脚本 / sidecar / 内容目录 / README ----
Step "组装迁移脚本与内容目录"
Copy-Item "$repo\ontology-service\migrations\*.sql" "$staging\migrations\ontology-service\"
Copy-Item "$repo\runtime-manager\migrations\*.sql"  "$staging\migrations\runtime-manager\"
Copy-Item "$repo\tools\rdf-sidecar\sidecar.py"      "$staging\tools\rdf-sidecar\"
Copy-Item -Recurse -Force "$repo\docs"               "$staging\docs"
Copy-Item -Recurse -Force "$repo\platform-knowledge" "$staging\platform-knowledge"
Copy-Item -Recurse -Force "$repo\research"           "$staging\research"
Copy-Item "$here\staging-assets\README.txt"          "$staging\README.txt"

# ---- 6. 中文语言包（官方 Unofficial ChineseSimplified.isl，缓存于 build\；缺失回退英文向导）----
$isl = "$build\ChineseSimplified.isl"
if (-not (Test-Path $isl)) {
    try {
        Step "下载中文语言包 ChineseSimplified.isl"
        Invoke-WebRequest -UseBasicParsing `
            -Uri "https://raw.githubusercontent.com/jrsoftware/issrc/main/Files/Languages/Unofficial/ChineseSimplified.isl" `
            -OutFile $isl
    } catch { Write-Host "[build] 中文语言包下载失败（安装向导回退英文）: $_" -ForegroundColor Yellow }
}

if ($StageOnly) { Step "StageOnly：staging 组装完成，跳过安装包编译"; exit 0 }

# ---- 7. 打包 setup.exe：ISCC（Inno Setup）优先，未安装则走 7-Zip SFX 兜底（零外部工具链）----
$iscc = $env:ISCC
if (-not $iscc) {
    foreach ($p in @(
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
    )) { if (Test-Path $p) { $iscc = $p; break } }
}

if ($iscc -and (Test-Path $iscc)) {
    Step "编译安装包（Inno Setup）：ISCC installer.iss"
    & $iscc /DMyAppVersion=$version $here\installer.iss
    if ($LASTEXITCODE -ne 0) { throw "ISCC 编译失败（exit $LASTEXITCODE）" }
} else {
    Step "未检测到 ISCC，走 7-Zip SFX 兜底路径（自举 7zr.exe + 7zSD.sfx）"
    # 工具链自举：7zr.exe（官方单文件）+ 7zSD.sfx（7z920-extra 经典安装器 SFX 模块，25.01 起已移除）
    if (-not (Test-Path "$cache\7zr.exe")) {
        Invoke-WebRequest -UseBasicParsing -Uri "https://www.7-zip.org/a/7zr.exe" -OutFile "$cache\7zr.exe"
    }
    if (-not (Test-Path "$cache\7zSD.sfx")) {
        Invoke-WebRequest -UseBasicParsing -Uri "https://www.7-zip.org/a/7z920_extra.7z" -OutFile "$cache\7z920-extra.7z"
        & "$cache\7zr.exe" e -y "$cache\7z920-extra.7z" 7zSD.sfx "-o$cache" | Out-Null
    }
    if (-not (Test-Path "$cache\7zSD.sfx")) { throw "7zSD.sfx 自举失败（检查网络或手动放置 $cache\7zSD.sfx）" }

    # sfx 组装目录：根放安装脚本，payload\ 收全部 staging 产物（被占用时改用固定备用名，
    # 不堆时间戳目录——旧 stage 残留会让「取最新目录」的调用方装到旧脚本）
    $sfxStage = "$build\sfx"
    try {
        if (Test-Path $sfxStage) { Remove-Item -Recurse -Force $sfxStage -ErrorAction Stop }
    } catch {
        $sfxStage = "$build\sfx-new"
        if (Test-Path $sfxStage) { Remove-Item -Recurse -Force $sfxStage -ErrorAction SilentlyContinue }
        Write-Host "[build] build\sfx 被占用，改用 $sfxStage" -ForegroundColor Yellow
    }
    New-Item -ItemType Directory -Force -Path "$sfxStage\payload" | Out-Null
    Move-Item $staging "$sfxStage\payload\app"
    # .cmd 批处理必须 CRLF（LF-only 会碎裂解析）；UTF-8 无 BOM + 首行 chcp 65001；
    # .ps1 需 UTF-8 BOM（PowerShell 5.1 中文兼容）
    $utf8 = New-Object System.Text.UTF8Encoding($false)
    $utf8bom = New-Object System.Text.UTF8Encoding($true)
    foreach ($f in @("installer.cmd", "uninstall.cmd")) {
        $c = [IO.File]::ReadAllText("$here\sfx\$f")
        $c = $c -replace "`r`n", "`n"
        $c = $c -replace "`n", "`r`n"
        [IO.File]::WriteAllText("$sfxStage\$f", $c, $utf8)
    }
    foreach ($f in @("install-shortcuts.ps1", "uninstall-core.ps1")) {
        $c = [IO.File]::ReadAllText("$here\sfx\$f")
        [IO.File]::WriteAllText("$sfxStage\$f", $c, $utf8bom)
    }
    # 原版 7zSD（7z920 extra）按 ANSI 经典配方：;!@Install@! 标记 + ASCII 配置
    # （中文标题交由 installer.cmd 内的 chcp 65001 输出承载，配置本体保持 ASCII 最稳）
    $cfg = ";!@Install@!`r`nTitle=`"MultiAgentLab Setup`"`r`nRunProgram=`"installer.cmd`"`r`n;!@InstallEnd@!`r`n"
    [IO.File]::WriteAllText("$sfxStage\config.txt", $cfg, [Text.Encoding]::ASCII)

    & "$cache\7zr.exe" a -mx=9 "$build\payload.7z" "$sfxStage\*" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "payload.7z 打包失败（exit $LASTEXITCODE）" }
    # SFX = 7zSD.sfx + config.txt + payload.7z（PowerShell 原生字节拼接，规避 cmd copy /b 多层引号丢参）
    $out = "$dist\MultiAgentLab_${version}_setup.exe"
    $p1 = [IO.File]::ReadAllBytes("$cache\7zSD.sfx")
    $p2 = [IO.File]::ReadAllBytes("$sfxStage\config.txt")
    $p3 = [IO.File]::ReadAllBytes("$build\payload.7z")
    $bytes = New-Object byte[] ($p1.Length + $p2.Length + $p3.Length)
    [Array]::Copy($p1, 0, $bytes, 0, $p1.Length)
    [Array]::Copy($p2, 0, $bytes, $p1.Length, $p2.Length)
    [Array]::Copy($p3, 0, $bytes, ($p1.Length + $p2.Length), $p3.Length)
    [IO.File]::WriteAllBytes($out, $bytes)
    if (-not (Test-Path $out)) { throw "SFX 拼接失败" }
    Remove-Item -Force "$build\payload.7z"
}

$setup = Get-ChildItem "$dist\MultiAgentLab_*_setup.exe" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
Step "完成：$($setup.FullName)（$([math]::Round($setup.Length/1MB,1)) MB）"
