#!/usr/bin/env bash
# MultiAgentLab macOS 桌面客户端构建（REQ-324/M127）
#
# 用法: deploy/macos/build.sh [-SkipWeb] [-SkipDmg] [-StageOnly]
#   默认       前端构建 + 三服务/编排器编译 + 组装 .app + ad-hoc 签名 + DMG
#   -SkipWeb   跳过前端构建（复用既有 web/dist）
#   -StageOnly 跳过全部编译（复用 build/bin 与 web/dist，仅重新组装/签名/DMG）
#   -SkipDmg   只出 .app 不打 DMG
#
# 产物: deploy/macos/dist/MultiAgentLab.app + dist/MultiAgentLab_<版本>.dmg
# 工具面: go + node（与 run-dev.sh 一致）——窗口壳为 AppleScriptObjC（系统 osascript），
# 零 cgo/零 Xcode CLT 依赖（正身机 CLT 损坏不影响构建/交付/运行）。
set -euo pipefail
cd "$(dirname "$0")"
MACOS_DIR="$PWD"
REPO="$(cd "$MACOS_DIR/../.." && pwd)"
VERSION="1.0.0"

SKIP_WEB=0; STAGE_ONLY=0; SKIP_DMG=0
for a in "$@"; do
  case "$a" in
    -SkipWeb) SKIP_WEB=1 ;;
    -StageOnly) STAGE_ONLY=1 ;;
    -SkipDmg) SKIP_DMG=1 ;;
    *) echo "未知参数: $a (支持 -SkipWeb/-SkipDmg/-StageOnly)"; exit 1 ;;
  esac
done

cd "$REPO"
if [ "$STAGE_ONLY" != "1" ]; then
  if [ "$SKIP_WEB" != "1" ]; then
    echo "[build] 前端构建..."
    (cd web && npm install --no-audit --no-fund && npm run build)
  fi
  echo "[build] 三服务 + 编排器编译..."
  mkdir -p "$MACOS_DIR/build/bin"
  (cd backend && go build -trimpath -o "$MACOS_DIR/build/bin/backend" ./cmd/backend)
  (cd ontology-service && go build -trimpath -o "$MACOS_DIR/build/bin/ontologyd" ./cmd/ontologyd)
  (cd runtime-manager && go build -trimpath -o "$MACOS_DIR/build/bin/runtimed" ./cmd/runtimed)
  (cd "$MACOS_DIR/launcher" && go build -trimpath -ldflags "-s -w" -o "$MACOS_DIR/build/bin/mlabd" .)
fi

echo "[build] 组装 MultiAgentLab.app..."
APP="$MACOS_DIR/dist/MultiAgentLab.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$MACOS_DIR/Info.plist" "$APP/Contents/Info.plist"
cp "$MACOS_DIR/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"  # REQ-321/324 待办销账：应用图标（品牌三节点网络）
RES="$APP/Contents/Resources"
mkdir -p "$RES/bin" "$RES/shell" "$RES/migrations" "$RES/web" "$RES/tools/rdf-sidecar"
cp "$MACOS_DIR/build/bin/backend" "$MACOS_DIR/build/bin/ontologyd" \
   "$MACOS_DIR/build/bin/runtimed" "$MACOS_DIR/build/bin/mlabd" "$RES/bin/"
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

echo "[build] ad-hoc 签名..."
codesign --force --deep --sign - "$APP" 2>/dev/null || echo "[build] 警告: ad-hoc 签名失败（本机直接运行通常不受影响）"

if [ "$SKIP_DMG" != "1" ]; then
  echo "[build] 打 DMG..."
  hdiutil create -volname MultiAgentLab -srcfolder "$APP" -ov -format UDZO \
    "$MACOS_DIR/dist/MultiAgentLab_${VERSION}.dmg" >/dev/null
fi

echo "[build] 完成:"
du -sh "$APP" "$MACOS_DIR/dist"/* 2>/dev/null | sed 's/^/  /'
