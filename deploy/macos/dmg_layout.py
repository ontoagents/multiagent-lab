#!/usr/bin/env python3
"""DMG 拖拽安装排版（对已挂载的 RW 镜像卷写 .DS_Store + 落背景图）。

用法: dmg_layout.py <mount_point> [bg_png]
卷内文件约定: MultiAgentLab.app / Applications(symlink → /Applications)。
依赖: pip install --user --break-system-packages ds_store mac_alias
（dmgbuild 一并安装即可带上；排版参数对齐 dmgbuild core.py 的 bwsp/icvp 字段集。）
"""
import os
import shutil
import sys

from ds_store import DSStore
from mac_alias import Alias

APP_NAME = "MultiAgentLab.app"
# 图标定位（对应背景图 660x400 的两个虚线槽位）
APP_POS = (140, 95)
APPS_POS = (520, 95)
WINDOW_RECT = ((100, 100), (660, 400))


def main():
    if len(sys.argv) < 2:
        sys.exit("用法: dmg_layout.py <mount_point> [bg_png]")
    mount = sys.argv[1]
    bg = sys.argv[2] if len(sys.argv) > 2 else ""

    for name in (APP_NAME, "Applications"):
        if not os.path.exists(os.path.join(mount, name)):
            sys.exit(f"卷内缺 {name}（排版前置不满足）")

    alias = None
    if bg and os.path.isfile(bg):
        dst = os.path.join(mount, ".background.png")
        shutil.copyfile(bg, dst)
        # alias 指向挂载卷内的真实文件（挂载态构造，卷名随 volname 命中）
        alias = Alias.for_file(dst)

    (x, y), (w, h) = WINDOW_RECT
    bwsp = {
        "ShowStatusBar": False,
        "WindowBounds": f"{{{{{x}, {y}}}, {{{w}, {h}}}}}",
        "ContainerShowSidebar": False,
        "PreviewPaneVisibility": False,
        "SidebarWidth": 0,
        "ShowTabView": False,
        "ShowToolbar": False,
        "ShowPathbar": False,
        "ShowSidebar": False,
    }
    icvp = {
        "viewOptionsVersion": 1,
        "backgroundType": 2 if alias else 1,
        "backgroundColorRed": 0.933,
        "backgroundColorGreen": 0.949,
        "backgroundColorBlue": 0.969,
        "gridOffsetX": 0.0,
        "gridOffsetY": 0.0,
        "gridSpacing": 100.0,
        "arrangeBy": "none",
        "showIconPreview": False,
        "showItemInfo": False,
        "labelOnBottom": True,
        "textSize": 13.0,
        "iconSize": 72.0,
        "scrollPositionX": 0.0,
        "scrollPositionY": 0.0,
    }
    if alias:
        icvp["backgroundImageAlias"] = alias.to_bytes()

    with DSStore.open(os.path.join(mount, ".DS_Store"), "w+") as d:
        d["."]["bwsp"] = bwsp
        d["."]["icvp"] = icvp
        d["."]["vstl"] = ("type", "icnv")  # vstl 无 codec 注册，须 (类型, 值) 二元组
        d[APP_NAME]["Iloc"] = APP_POS
        d["Applications"]["Iloc"] = APPS_POS
    print(f"[layout] .DS_Store 写入完成（background={'图' if alias else '纯色'}）")


if __name__ == "__main__":
    main()
