// DPI 感知与窗口尺寸缩放（REQ-321：高缩放屏〔125%/150%〕下 WebView2 窗口模糊的修复）。
//
// 根因：go-webview2 不处理 DPI，Go 进程默认 DPI-unaware——Windows 在高缩放屏上把整个窗口
// 按位图拉伸渲染，文字/界面全部模糊。进程在创建任何窗口前声明 per-monitor DPI awareness
// 即可让 WebView2 以原生分辨率合成（清晰）；窗口尺寸需按 DPI 缩放（物理像素 = 逻辑 × DPI/96）。
//
// 本文件在 launcher 与 setup 两个独立 module 各有一份同源拷贝（跨 module 不共享 internal 包）；
// 修改时请同步。v1 边界：跨显示器拖动时的 WM_DPICHANGED 动态重排未处理（按创建时 DPI 定尺寸）。
package main

import (
	"golang.org/x/sys/windows"
)

var (
	shcore          = windows.NewLazySystemDLL("shcore.dll")
	procSetDpiAware = shcore.NewProc("SetProcessDpiAwareness")
	user32          = windows.NewLazySystemDLL("user32.dll")
	procSetDPIAware = user32.NewProc("SetProcessDPIAware")
	procGetDpiSys   = user32.NewProc("GetDpiForSystem")
	procGetDC       = user32.NewProc("GetDC")
	procReleaseDC   = user32.NewProc("ReleaseDC")
	procGetDevCaps  = user32.NewProc("GetDeviceCaps")
)

// setProcessDpiAware 声明 per-monitor DPI 感知（Win10 1703+ 的 shcore 路径；失败回退
// user32 SetProcessDPIAware〔system-aware，多屏同缩放场景等效〕）。必须在创建任何窗口前调用。
func setProcessDpiAware() {
	const processPerMonitorDpiAware = 2
	if procSetDpiAware.Find() == nil {
		_, _, _ = procSetDpiAware.Call(processPerMonitorDpiAware)
		return
	}
	if procSetDPIAware.Find() == nil {
		_, _, _ = procSetDPIAware.Call()
	}
}

// systemDPI 系统 DPI（96=100% 缩放）。GetDpiForSystem（Win10+）优先，回退 GetDeviceCaps(LOGPIXELSY)。
func systemDPI() int {
	if procGetDpiSys.Find() == nil {
		if r, _, _ := procGetDpiSys.Call(); r != 0 {
			return int(r)
		}
	}
	if procGetDC.Find() == nil {
		dc, _, _ := procGetDC.Call(0)
		if dc != 0 {
			const logPixelsY = 90
			r, _, _ := procGetDevCaps.Call(dc, logPixelsY)
			procReleaseDC.Call(0, dc)
			if r != 0 {
				return int(r)
			}
		}
	}
	return 96
}

// scaleSize 逻辑尺寸（96 DPI 基准）→ 物理像素。
func scaleSize(w, h, dpi int) (int, int) {
	return w * dpi / 96, h * dpi / 96
}
