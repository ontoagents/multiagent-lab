# REQ-321 Windows 桌面安装包构建脚本（PowerShell 5.1+）
# 用法：
#   powershell -ExecutionPolicy Bypass -File build.ps1                 # 全量：三服务+壳+前端+staging+setup.exe
#   powershell -ExecutionPolicy Bypass -File build.ps1 -SkipWeb        # 跳过前端构建（web/dist 未变时）
#   powershell -ExecutionPolicy Bypass -File build.ps1 -StageOnly      # 只组装 staging，不编译安装包
# 产物：deploy/windows/dist/MultiAgentLab_<版本>_setup.exe（per-user 免管理员安装）
# 依赖：Go 1.25+、Node 22+、ISCC（Inno Setup 6；env ISCC 或默认安装路径自动发现）

param(
    [string]$Shell = "tauri",    # tauri（REQ-321 定案壳技术，NSIS）| go（Go+WebView2 遗留壳，SFX/ISCC）
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

# ---- 2. 客户端壳（tauri=定案壳技术；go=遗留 Go+WebView2 壳）----
if ($Shell -eq "go") {
    Step "编译客户端壳 MultiAgentLab.exe（Go+WebView2 遗留壳）"
    Push-Location "$here\launcher"
    $env:GOWORK = "off"; go build -trimpath -ldflags "-s -w -H windowsgui" -o "$staging\MultiAgentLab.exe" .
    Remove-Item Env:\GOWORK; Pop-Location
}

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

# ---- 7. Tauri 壳打包（-Shell tauri：NSIS per-user，REQ-321 定案壳技术）----
if ($Shell -eq "tauri") {
    Step "组装 Tauri 资源（src-tauri\resources\：web-dist/migrations/docs/平台知识；bin\ 外部三服务）"
    $tsrc = "$repo\apps\desktop\src-tauri"
    $tres = "$tsrc\resources"
    # 整目录资源（web-dist/docs/platform-knowledge/research）不预建目录：Copy-Item -Recurse 的目标目录
    # 已存在时会把源目录整个拷进去（web-dist\dist\ 双层嵌套 → WEB_DIST 指向处无 index.html，/ 404），
    # 目标缺席时才是「拷贝并改名」语义
    foreach ($d in @("resources\web-dist", "resources\docs", "resources\platform-knowledge", "resources\research")) {
        if (Test-Path "$tsrc\$d") { Remove-Item -Recurse -Force "$tsrc\$d" }
    }
    foreach ($d in @("resources\migrations\ontology-service", "resources\migrations\runtime-manager",
                     "resources\tools\rdf-sidecar", "bin")) {
        if (Test-Path "$tsrc\$d") { Remove-Item -Recurse -Force "$tsrc\$d" }
        New-Item -ItemType Directory -Force -Path "$tsrc\$d" | Out-Null
    }
    Copy-Item -Recurse -Force "$repo\web\dist" "$tres\web-dist"
    Copy-Item "$repo\ontology-service\migrations\*.sql" "$tres\migrations\ontology-service\"
    Copy-Item "$repo\runtime-manager\migrations\*.sql"  "$tres\migrations\runtime-manager\"
    Copy-Item "$repo\tools\rdf-sidecar\sidecar.py"      "$tres\tools\rdf-sidecar\"
    Copy-Item -Recurse -Force "$repo\docs"               "$tres\docs"
    Copy-Item -Recurse -Force "$repo\platform-knowledge" "$tres\platform-knowledge"
    Copy-Item -Recurse -Force "$repo\research"           "$tres\research"
    Copy-Item "$staging\bin\backend.exe"    "$tsrc\bin\backend-x86_64-pc-windows-msvc.exe"
    Copy-Item "$staging\bin\ontologyd.exe"  "$tsrc\bin\ontologyd-x86_64-pc-windows-msvc.exe"
    Copy-Item "$staging\bin\runtimed.exe"   "$tsrc\bin\runtimed-x86_64-pc-windows-msvc.exe"

    # Tauri CLI 自举（node_modules 不入库：CI/新机首次构建时 npx tauri 无可执行文件）；
    # package-lock.json 在库，npm ci 可复现
    if (-not (Test-Path "$repo\apps\desktop\node_modules\@tauri-apps\cli\package.json")) {
        Step "安装 Tauri CLI（apps/desktop npm ci）"
        Push-Location "$repo\apps\desktop"; npm ci --no-audit --no-fund; Pop-Location
        if ($LASTEXITCODE -ne 0) { throw "apps/desktop npm ci 失败（exit $LASTEXITCODE）" }
    }

    Step "tauri build（NSIS per-user 安装包，首次编译 Rust 依赖需较长时间）"
    Push-Location "$repo\apps\desktop"
    # npx 的 stderr notice 会触发 PS5.1 ErrorActionPreference=Stop 误判，经 cmd 层合并流
    cmd /c "npx tauri build --bundles nsis 2>&1"
    if ($LASTEXITCODE -ne 0) { Pop-Location; throw "tauri build 失败（exit $LASTEXITCODE）" }
    Pop-Location
    $nsis = Get-ChildItem "$tsrc\target\release\bundle\nsis\*setup.exe" -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $nsis) { throw "Tauri NSIS 产物缺失（检查 tauri build 输出）" }
    Copy-Item $nsis.FullName "$dist\" -Force
    Step "完成：$($nsis.FullName)（$([math]::Round($nsis.Length/1MB,1)) MB）"
    exit 0
}

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
    Step "未检测到 ISCC，走 Go 自解压引导器兜底路径"
    # 工具链自举：setup-bootstrap.exe（deploy/windows/setup 的 Go 引导器缓存；首次自动编译）

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
    # setup.exe = Go 自解压引导器（deploy/windows/setup，读自身尾部附加 zip）+ Compress-Archive 生成的
    # payload.zip 拼接。弃用 7zSD.sfx：其配置标记兼容性黑盒（ANSI 标记走「找 setup.exe」默认行为，
    # 用户双击报「找不到 setup.exe」；对话框流程在本机无法可视化验证），自研引导器全程确定性可控。
    # Go 引导器编译（GOWORK=off 独立 module）
    if (-not (Test-Path "$cache\setup-bootstrap.exe")) {
        Push-Location "$here\setup"
        # -H windowsgui：双击安装只出现向导窗口，不弹控制台黑窗（进度在向导页内）
        $env:GOWORK = "off"; go build -trimpath -ldflags "-s -w -H windowsgui" -o "$cache\setup-bootstrap.exe" .
        Remove-Item Env:\GOWORK; Pop-Location
        if ($LASTEXITCODE -ne 0) { throw "setup 引导器编译失败" }
    }
    Step "打包 payload.zip（Compress-Archive）"
    $zip = "$build\payload.zip"
    if (Test-Path $zip) { Remove-Item -Force $zip }
    Compress-Archive -Path "$sfxStage\*" -DestinationPath $zip -CompressionLevel Optimal
    if (-not (Test-Path $zip)) { throw "payload.zip 打包失败" }
    Step "拼接 setup.exe = 引导器 + payload.zip"
    $out = "$dist\MultiAgentLab_${version}_setup.exe"
    $p1 = [IO.File]::ReadAllBytes("$cache\setup-bootstrap.exe")
    $p3 = [IO.File]::ReadAllBytes($zip)
    $bytes = New-Object byte[] ($p1.Length + $p3.Length)
    [Array]::Copy($p1, 0, $bytes, 0, $p1.Length)
    [Array]::Copy($p3, 0, $bytes, $p1.Length, $p3.Length)
    [IO.File]::WriteAllBytes($out, $bytes)
    if (-not (Test-Path $out)) { throw "setup 拼接失败" }
    Remove-Item -Force $zip
}

$setup = Get-ChildItem "$dist\MultiAgentLab_*_setup.exe" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
Step "完成：$($setup.FullName)（$([math]::Round($setup.Length/1MB,1)) MB）"
