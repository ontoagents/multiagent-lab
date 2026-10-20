#!/usr/bin/env bash
# MultiAgentLab macOS 桌面客户端构建（REQ-324/M127；壳技术切换轮 2026-10-15 增 Tauri 壳路径）
#
# 用法: deploy/macos/build.sh [-Shell tauri|go] [-SkipWeb] [-SkipDmg] [-StageOnly]
#   -Shell tauri  Tauri 2 壳（定案壳技术，对齐 build.ps1 默认）：组 apps/desktop 资源 →
#                 npx tauri build（.app）→ ad-hoc 签名 → hdiutil DMG。产物单架构（本机 triple），
#                 universal 挂 CI 迁移轮（Rust 双架构+lipo，见 18 号迭代行诚实边界）。
#   -Shell go     Go+AppleScriptObjC 遗留壳（REQ-324 首交付形态；client-release.yml workflow 沿用）
#   默认 -Shell tauri；-SkipWeb 跳过前端构建；-StageOnly 只组装不打包；-SkipDmg 只出 .app
#
# 产物: dist/MultiAgentLab.app + dist/MultiAgentLab_<版本>.dmg（两种壳同名落位）
#
# 工具面:
#   go 壳   = go + node（零 Xcode CLT 依赖，正身机 CLT 损坏不影响）
#   tauri 壳 = go + node + Rust（rustup stable）+ 链接驱动——
#             CLT 完好机器走系统 clang；CLT 损坏机器自动回落 zig cc（MLAB_ZIG_WRAPPER 可覆盖
#             wrapper 路径，wrapper 内 -isysroot 指向残存 SDK）——见 20 号 S9 边界注。
set -euo pipefail
cd "$(dirname "$0")"
MACOS_DIR="$PWD"
REPO="$(cd "$MACOS_DIR/../.." && pwd)"

# ---- 版本口径（2026-10-15 定）：主版本 0；次版本随每次构建递增（.build-number 计数器，
# gitignored——换机从 1 重计不追历史）；后缀=构建时 git 提交 id short。形如 0.3-425f096。
BUILD_NUM_FILE="$MACOS_DIR/.build-number"
PREV_NUM="$(cat "$BUILD_NUM_FILE" 2>/dev/null || echo 0)"
BUILD_NUM=$((PREV_NUM + 1))
SHORT_ID="$(git -C "$REPO" rev-parse --short HEAD 2>/dev/null || echo unknown)"
VERSION_ID="0.${BUILD_NUM}-${SHORT_ID}"
# semver 形态（tauri.conf version 校验须三段；prerelease 段即提交 id）：0.3.0-425f096
VERSION_SEMVER="0.${BUILD_NUM}.0-${SHORT_ID}"
echo "[build] 版本: ${VERSION_ID}（semver ${VERSION_SEMVER}；主版本 0 · 次版本每次成功构建 +1 · 后缀=提交 id）"
PY3="${MLAB_PY3:-/usr/local/bin/python3.12}"

SHELL_KIND="tauri"; SKIP_WEB=0; STAGE_ONLY=0; SKIP_DMG=0; PLAIN_DMG=0
while [ $# -gt 0 ]; do
  case "$1" in
    -Shell) SHELL_KIND="$2"; shift 2 ;;
    -SkipWeb) SKIP_WEB=1; shift ;;
    -StageOnly) STAGE_ONLY=1; shift ;;
    -SkipDmg) SKIP_DMG=1; shift ;;
    -PlainDmg) PLAIN_DMG=1; shift ;;
    *) echo "未知参数: $1 (支持 -Shell tauri|go / -SkipWeb / -SkipDmg / -StageOnly / -PlainDmg)"; exit 1 ;;
  esac
done
[ "$SHELL_KIND" = "tauri" ] || [ "$SHELL_KIND" = "go" ] || { echo "-Shell 只支持 tauri|go"; exit 1; }

cd "$REPO"

if [ "$STAGE_ONLY" != "1" ]; then
  if [ "$SKIP_WEB" != "1" ]; then
    echo "[build] 前端构建..."
    (cd web && npm install --no-audit --no-fund && npm run build)
  fi
  echo "[build] 三服务编译（$(uname -m)）..."
  mkdir -p "$MACOS_DIR/build/bin"
  (cd backend && go build -trimpath -o "$MACOS_DIR/build/bin/backend" ./cmd/backend)
  (cd ontology-service && go build -trimpath -o "$MACOS_DIR/build/bin/ontologyd" ./cmd/ontologyd)
  (cd runtime-manager && go build -trimpath -o "$MACOS_DIR/build/bin/runtimed" ./cmd/runtimed)
  # ---- oxigraph 引擎内置（REQ-321 零捆绑口径 2026-10-15 反转）----
  # 官方 release v0.5.11 双架构下载 + lipo 合成 universal（Intel/Apple Silicon 一包双跑）；
  # 单架构下载或 lipo 不可用（CLT 损坏机）回落本机可达架构；全败则不内置（首启一键安装兜底）。
  OXI_BIN="$MACOS_DIR/build/bin/oxigraph"
  if [ ! -x "$OXI_BIN" ]; then
    OXI_BASE="https://github.com/oxigraph/oxigraph/releases/download/v0.5.11"
    OXI_TMP="$(mktemp -d)"
    fetch_oxi() {  # $1=aarch64|x86_64 → echo 解包出的 oxigraph_server 路径（失败返非零）
      curl -fsSL -o "$OXI_TMP/$1.tgz" "$OXI_BASE/oxigraph_v0.5.11_${1}_apple.tar.gz" || return 1
      mkdir -p "$OXI_TMP/$1" && tar -xzf "$OXI_TMP/$1.tgz" -C "$OXI_TMP/$1" || return 1
      find "$OXI_TMP/$1" -type f -name oxigraph_server | head -1
    }
    A64="$(fetch_oxi aarch64 || true)"
    X64="$(fetch_oxi x86_64 || true)"
    if [ -n "$A64" ] && [ -n "$X64" ] && lipo -create -output "$OXI_BIN" "$A64" "$X64" 2>/dev/null; then
      echo "[build] oxigraph universal（aarch64+x86_64）内置就绪"
    elif [ -n "$A64" ] && cp "$A64" "$OXI_BIN"; then
      echo "[build] oxigraph 仅 aarch64 内置（x86_64 下载/lipo 失败，Intel 机首启一键安装兜底）"
    elif [ -n "$X64" ] && cp "$X64" "$OXI_BIN"; then
      echo "[build] oxigraph 仅 x86_64 内置"
    else
      echo "[build] 警告: oxigraph 下载失败，安装包不含内置引擎（首启运行页一键安装兜底）"
    fi
    rm -rf "$OXI_TMP"
  fi
  [ -f "$OXI_BIN" ] && chmod +x "$OXI_BIN"
fi


# ---- 签名助手（公证就绪形态，2026-10-20）：MACOS_SIGN_IDENTITY 配置即用 Developer ID 签名
#（--options runtime 强化运行时为公证前置要求）；未配置维持 ad-hoc（收包人需清隔离，见 README）。
sign_app() {
  if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
    echo "[build] Developer ID 签名（${MACOS_SIGN_IDENTITY}，hardened runtime）..."
    codesign --force --deep --options runtime --timestamp --sign "$MACOS_SIGN_IDENTITY" "$1"
  else
    codesign --force --deep --sign - "$1" 2>/dev/null || true
  fi
}

if [ "$SHELL_KIND" = "go" ]; then
  # ---- 遗留壳（REQ-324 首交付形态：ASOC + WKWebView，零编译器依赖）----
  if [ "$STAGE_ONLY" != "1" ]; then
    echo "[build] 编排器 mlabd 编译..."
    (cd "$MACOS_DIR/launcher" && go build -trimpath -ldflags "-s -w -X main.ShellVersion=${VERSION_ID}" -o "$MACOS_DIR/build/bin/mlabd" .)
  fi

  echo "[build] 组装 MultiAgentLab.app（go/ASOC 壳）..."
  APP="$MACOS_DIR/dist/MultiAgentLab.app"
  rm -rf "$APP"
  mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
  cp "$MACOS_DIR/Info.plist" "$APP/Contents/Info.plist"
  # 版本注入（CFBundleShortVersionString/CFBundleVersion 两处同源 1.0.0 模板值）
  sed -i '' "s|<string>1.0.0</string>|<string>${VERSION_SEMVER}</string>|g" "$APP/Contents/Info.plist"
  RES="$APP/Contents/Resources"
  mkdir -p "$RES/bin" "$RES/shell" "$RES/migrations" "$RES/web" "$RES/tools/rdf-sidecar"
  cp "$MACOS_DIR/build/bin/backend" "$MACOS_DIR/build/bin/ontologyd" \
     "$MACOS_DIR/build/bin/runtimed" "$MACOS_DIR/build/bin/mlabd" "$RES/bin/"
  [ -f "$MACOS_DIR/build/bin/oxigraph" ] && cp "$MACOS_DIR/build/bin/oxigraph" "$RES/bin/"
  cp "$MACOS_DIR/launcher.sh" "$APP/Contents/MacOS/MultiAgentLab"
  cp "$MACOS_DIR/shell/window.applescript" "$RES/shell/window.applescript"
  chmod +x "$APP/Contents/MacOS/MultiAgentLab" "$RES/bin/"*
  cp -R "$REPO/web/dist" "$RES/web/dist"
  cp -R "$REPO/ontology-service/migrations" "$RES/migrations/ontology-service"
  cp -R "$REPO/runtime-manager/migrations" "$RES/migrations/runtime-manager"
  cp -R "$REPO/docs" "$RES/docs"
  cp -R "$REPO/research" "$RES/research"
  cp -R "$REPO/platform-knowledge" "$RES/platform-knowledge"
  cp "$REPO/tools/rdf-sidecar/sidecar.py" "$RES/tools/rdf-sidecar/sidecar.py"
  cp "$MACOS_DIR/staging-assets/README.txt" "$RES/README.txt"

  sign_app "$APP"
else
  # ---- Tauri 2 壳（定案壳技术，对齐 deploy/windows/build.ps1 -Shell tauri 资源布局）----
  if [ -n "${GOARCH:-}" ] && [ "$GOARCH" != "$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" ]; then
    echo "[build] 错误: tauri 壳本轮仅支持本机架构（GOARCH=$GOARCH 交叉编译挂 CI 迁移轮）"; exit 1
  fi
  TSRC="$REPO/apps/desktop/src-tauri"
  TRES="$TSRC/resources"
  echo "[build] 组装 Tauri 资源（src-tauri/resources + bin/<triple>）..."
  rm -rf "$TRES" "$TSRC/bin"
  mkdir -p "$TRES/migrations/ontology-service" "$TRES/migrations/runtime-manager" \
           "$TRES/tools/rdf-sidecar" "$TSRC/bin"
  cp -R "$REPO/web/dist" "$TRES/web-dist"
  cp "$REPO/ontology-service/migrations/"*.sql "$TRES/migrations/ontology-service/"
  cp "$REPO/runtime-manager/migrations/"*.sql "$TRES/migrations/runtime-manager/"
  cp "$REPO/tools/rdf-sidecar/sidecar.py" "$TRES/tools/rdf-sidecar/"
  cp -R "$REPO/docs" "$TRES/docs"
  cp -R "$REPO/platform-knowledge" "$TRES/platform-knowledge"
  cp -R "$REPO/research" "$TRES/research"
  TRIPLE="$(rustc -vV 2>/dev/null | awk '/^host/{print $2}')"
  [ -n "$TRIPLE" ] || TRIPLE="$(uname -m)-apple-darwin"
  for svc in backend ontologyd runtimed oxigraph; do
    [ -f "$MACOS_DIR/build/bin/$svc" ] || { echo "[build] 错误: 缺 ${svc}（-StageOnly 需先全量构建；oxigraph 由全量构建期官方 release 下载内置）"; exit 1; }
    cp "$MACOS_DIR/build/bin/$svc" "$TSRC/bin/$svc-$TRIPLE"
  done

  if [ "$STAGE_ONLY" != "1" ]; then
    echo "[build] tauri build（.app；首次编译 Rust 依赖需较长时间）..."
    # 链接驱动：CLT 完好走系统 clang；CLT 损坏（本机）自动回落「rust-lld 直链 + zig cc 编译 C」——
    # zig 0.13 自带 Mach-O 链接器不支持 -exported_symbols_list（proc-macro 必需），故链接必须走 ld64.lld
    if ! clang --version >/dev/null 2>&1; then
      ZW="${MLAB_ZIG_WRAPPER:-$HOME/tools/zig-cc-wrapper.sh}"
      LLD="$HOME/tools/ld64.lld"
      RUSTL="$(ls "$HOME/.rustup/toolchains/"stable-*/lib/rustlib/*/bin/rust-lld 2>/dev/null | head -1 || true)"
      SDK="/Library/Developer/CommandLineTools/SDKs/MacOSX12.1.sdk"
      if [ ! -x "$ZW" ] || [ -z "$RUSTL" ] || [ ! -d "$SDK" ]; then
        echo "[build] 错误: CLT 不可用且 zig wrapper（${ZW}）/rust-lld（${RUSTL}）/SDK（${SDK}）任一缺失"; exit 1
      fi
      ln -sf "$RUSTL" "$LLD"
      TRIPLE="$(rustc -vV 2>/dev/null | awk '/^host/{print $2}')"
      mkdir -p "$TSRC/.cargo"
      cat > "$TSRC/.cargo/config.toml" <<EOF
# 本文件由 deploy/macos/build.sh 生成（机器相关，gitignored）：CLT 损坏机的链接降级面
[target.$TRIPLE]
linker = "$LLD"
rustflags = ["-C", "linker-flavor=ld64.lld", "-C", "link-arg=-syslibroot", "-C", "link-arg=$SDK"]
EOF
      echo "[build] CLT 不可用 → 链接=rust-lld（${LLD}）+ C 编译=zig cc（${ZW}）"
      export CC="$ZW"
      export CXX="$ZW"
      # CLT 掏空无 ar/ranlib（cc-rs 打静态库需要）→ zig 内置 llvm-ar/ranlib shim
      export PATH="$HOME/tools/bin:$PATH"
      export AR="$HOME/tools/bin/ar"
      export RANLIB="$HOME/tools/bin/ranlib"
    fi
    (cd "$REPO/apps/desktop" && npm install --no-audit --no-fund >/dev/null 2>&1 || true)
    # 版本注入：tauri.conf.json version 覆盖（semver 形如 0.3-425f096，prerelease 段即提交 id）
    VCONF="$(mktemp -t mlabver)"
    printf '{"version": "%s"}' "$VERSION_SEMVER" > "$VCONF"
    (cd "$REPO/apps/desktop" && npx tauri build --bundles app -c "$VCONF")
    rm -f "$VCONF"
    BUILT="$(ls -d "$TSRC/target/release/bundle/macos/MultiAgentLab.app" 2>/dev/null || true)"
    [ -n "$BUILT" ] || { echo "[build] 错误: tauri 产物缺失"; exit 1; }
    mkdir -p "$MACOS_DIR/dist"
    rm -rf "$MACOS_DIR/dist/MultiAgentLab.app"
    cp -R "$BUILT" "$MACOS_DIR/dist/MultiAgentLab.app"
    sign_app "$MACOS_DIR/dist/MultiAgentLab.app"
  fi
fi

# DMG 段只受 -SkipDmg 控制（-StageOnly=跳过编译、仍打包）——client-release.yml macos job 三段式
# 的第三段依赖本语义〔lipo 通用二进制 → -StageOnly 重组装 + DMG → mv 注入版本号〕；
# 勿再收窄（ff8ce92 曾误加 STAGE_ONLY 条件致 workflow mv 无 DMG 可改名，2026-10-19 修复）。
if [ "$SKIP_DMG" != "1" ]; then
  APP="$MACOS_DIR/dist/MultiAgentLab.app"
  DMG_OUT="$MACOS_DIR/dist/MultiAgentLab_${VERSION_ID}.dmg"
  LAYOUT_OK=0
  # ---- DMG 拖拽安装排版（默认）：应用图标 + /Applications 别名 + 背景引导 + 固定窗口布局。
  # 管线=hdiutil UDRW -srcfolder（copyfile 保字节名，绕开 shutil 触发 HFS+ 分解致 seal 破损的坑）
  # → 挂 RW → dmg_layout.py 写 .DS_Store（ds_store+mac_alias，零访达自动化权限）→ 卸载 → UDZO 转换
  # → 挂只读 codesign 校验硬闸。降级链：无 Chrome→无背景纯排版；缺 ds_store/排版或校验失败→纯镜像
  #（-PlainDmg 显式退回）。
  LAYOUT_OK=0
  if [ "$PLAIN_DMG" != "1" ] && "$PY3" -c 'import ds_store, mac_alias' >/dev/null 2>&1; then
    echo "[build] 打 DMG（拖拽安装排版）..."
    STAGE="$(mktemp -d)"
    RW="$(mktemp -d)/rw.dmg"
    MNT="$(mktemp -d)"
    cp -R "$APP" "$STAGE/MultiAgentLab.app"
    ln -s /Applications "$STAGE/Applications"
    hdiutil create -volname MultiAgentLab -srcfolder "$STAGE" -ov -format UDRW "$RW" >/dev/null
    if hdiutil attach -nobrowse -readwrite -mountpoint "$MNT" "$RW" >/dev/null; then
      BG=""
      CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
      if [ -x "$CHROME" ]; then
        BGHTML="$(mktemp).html"
        cat > "$BGHTML" <<'HTMLEOF'
<!doctype html><html><head><meta charset="utf-8"><style>
body{margin:0;width:660px;height:400px;background:linear-gradient(135deg,#eef3fb 0%,#dfe9f7 60%,#d3e2f5 100%);
font-family:-apple-system,'PingFang SC',sans-serif;overflow:hidden}
.arrow{position:absolute;top:44px;left:265px;font-size:120px;color:#3a6fd8;font-weight:300}
.t1{position:absolute;top:196px;width:660px;text-align:center;font-size:26px;color:#1f3a68;font-weight:600}
.t2{position:absolute;top:246px;width:660px;text-align:center;font-size:16px;color:#5a719c}
.slot{position:absolute;border:2.5px dashed #9db8e0;border-radius:18px}
.s1{top:30px;left:75px;width:130px;height:130px}
.s2{top:30px;left:455px;width:130px;height:130px}
.cap{position:absolute;width:280px;text-align:center;font-size:14px;color:#6d84ad}
.c1{top:196px;left:0}.c2{top:196px;left:380px}
</style></head><body>
<div class="slot s1"></div><div class="slot s2"></div>
<div class="cap c1">MultiAgentLab</div><div class="cap c2">Applications</div>
<div class="arrow">→</div>
<div class="t1">拖动「MultiAgentLab」到右侧 Applications</div>
<div class="t2">完成永久安装（也可拖入其他位置；数据独立存放不受影响）</div>
</body></html>
HTMLEOF
        BG="$(mktemp).png"
        if ! "$CHROME" --headless=new --disable-gpu --window-size=660,400 --screenshot="$BG" "file://$BGHTML" >/dev/null 2>&1 || [ ! -s "$BG" ]; then
          echo "[build] 警告: 背景图渲染失败 → 排版降级纯色背景"
          BG=""
        fi
      else
        echo "[build] 警告: 未找到 Chrome → 排版降级纯色背景"
      fi
      if "$PY3" "$MACOS_DIR/dmg_layout.py" "$MNT" "$BG"; then
        LAYOUT_OK=1
      else
        echo "[build] 警告: 排版写入失败 → 回落纯镜像 DMG"
      fi
      hdiutil detach "$MNT" >/dev/null 2>&1 || { sleep 2; hdiutil detach -force "$MNT" >/dev/null 2>&1 || true; }
      if [ "$LAYOUT_OK" = "1" ]; then
        hdiutil convert "$RW" -format UDZO -o "$DMG_OUT" >/dev/null
      fi
      rm -rf "$STAGE" "$RW"
    else
      echo "[build] 警告: RW 镜像挂载失败 → 回落纯镜像 DMG"
    fi
    [ -n "${BGHTML:-}" ] && [ -f "$BGHTML" ] && rm -f "$BGHTML"
    [ -n "${BG:-}" ] && [ -f "$BG" ] && rm -f "$BG"
  else
    if [ "$PLAIN_DMG" != "1" ]; then
      echo "[build] 警告: 缺 ds_store/mac_alias（$PY3 -m pip install --user --break-system-packages dmgbuild）→ 回落纯镜像 DMG"
    fi
  fi
  if [ "$LAYOUT_OK" != "1" ]; then
    echo "[build] 打 DMG（纯镜像）..."
    hdiutil create -volname MultiAgentLab -srcfolder "$APP" -ov -format UDZO "$DMG_OUT" >/dev/null
  fi
  # ---- 在镜像签名校验硬闸（防跨 FS 封印破损流出到用户）：挂只读 codesign 验证，失败即构建失败
  VNT="$(mktemp -d)"
  if hdiutil attach -nobrowse -readonly -mountpoint "$VNT" "$DMG_OUT" >/dev/null; then
    if codesign --verify --deep --strict "$VNT/MultiAgentLab.app" 2>/dev/null; then
      echo "[build] DMG 内签名校验通过（${VERSION_ID}）"
    else
      echo "[build] 错误: DMG 内签名校验失败（seal 破损）"; hdiutil detach "$VNT" >/dev/null 2>&1 || true; exit 1
    fi
    hdiutil detach "$VNT" >/dev/null
  else
    echo "[build] 警告: DMG 校验挂载失败（跳过在镜像签名断言）"
  fi
fi

# ---- 公证 + staple（公证就绪形态）：NOTARY_PROFILE=已存储的 notarytool keychain profile
#（一次性配置：xcrun notarytool store-credentials <profile> --apple-id … --team-id … --password …，
#  或 API 密钥形态）。未配置跳过——收包人按 README 清隔离即可。
if [ -n "${NOTARY_PROFILE:-}" ] && [ "${SKIP_DMG:-}" != "1" ] && [ "${STAGE_ONLY:-}" != "1" ] && [ -f "${DMG_OUT:-}" ]; then
  echo "[build] 公证提交（notarytool，可能持续数分钟）..."
  if xcrun notarytool submit "$DMG_OUT" --keychain-profile "$NOTARY_PROFILE" --wait; then
    xcrun stapler staple "$DMG_OUT"
    echo "[build] 公证+staple 完成——收包人下载后可直接打开"
  else
    echo "[build] 错误: 公证失败（DMG 保留未 staple 态供排查）"; exit 1
  fi
fi

# 计数器只在构建成功后落盘（失败尝试不消耗次版本号）
echo "$BUILD_NUM" > "$BUILD_NUM_FILE"
echo "[build] 完成（shell=${SHELL_KIND}）:"
du -sh "$MACOS_DIR/dist"/MultiAgentLab.app "$MACOS_DIR/dist"/*.dmg 2>/dev/null | sed 's/^/  /' || true
