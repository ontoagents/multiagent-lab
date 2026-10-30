-- OntoAgents macOS 桌面客户端窗口壳（REQ-324/M127）。
--
-- AppleScriptObjC + WKWebView：由 bundle 脚本（Contents/MacOS/OntoAgents）exec 本脚本
-- （同一进程承载 NSApplication，单 Dock 图标）；服务编排由后台 mlabd 承担，本壳职责：
--   - 等主平台就绪（轮询 /healthz，≤30s；超时弹窗示因 + 兜底 -stop）；
--   - 建窗口（WKWebView 直载 MLAB_URL，缺省 http://127.0.0.1:8180〔REQ-332 默认段〕，与 Windows WebView2 壳同构）；
--   - 菜单栏：App 菜单（退出 Cmd+Q）+ 编辑菜单（撤销/重做/剪切/拷贝/粘贴/全选——
--     无此菜单 WKWebView 文本框 Cmd+C/V 失效）+ 窗口菜单（最小化/关闭）；
--   - 关窗（红点/Cmd+W）→ applicationShouldTerminate → mlabd -stop 整树回收
--     （mlabd 侧另有 ppid 监视兜底：壳被强杀亦回收）。
--
-- 环境变量（bundle 脚本注入）：MLAB_URL / MLAB_HEALTH_URL / MLAB_APP_NAME /
-- MLAB_WIN_W / MLAB_WIN_H / MLAB_MLABD / MLAB_SHELL_LOG（调试日志，缺省静默）。
--
-- ASOC 注意（实测 osascript 环境）：方法调用须下划线形（Xxx_yyy_）；零参与术语冲突的
-- 选择器（center/run）经 NSSelectorFromString+performSelector_ 派发；NSRect 用 NSMakeRect
-- 构造（record 字面量无法强转 CGRect）；`use scripting additions` 必须显式声明。

use framework "Foundation"
use framework "AppKit"
use framework "WebKit"
use scripting additions

global gWin, gWV, gApp, gMlabd, gStopping

on logLine(s)
	if MLAB_SHELL_LOG() is not "" then
		try
			do shell script "echo " & quoted form of s & " >> " & quoted form of MLAB_SHELL_LOG()
		end try
	end if
end logLine

on envVal(k, dflt)
	set v to (do shell script "echo ${" & k & "-}" )
	if v is "" then return dflt
	return v
end envVal

on MLAB_SHELL_LOG()
	return envVal("MLAB_SHELL_LOG", "")
end MLAB_SHELL_LOG

on dialogText(t)
	display dialog t with title (envVal("MLAB_APP_NAME", "智能体构建平台")) buttons {"好"} default button 1
end dialogText

on windowWillClose_(notification)
	logLine("windowWillClose")
	if gStopping then return
	set gStopping to true
	gApp's terminate_(me)
end windowWillClose_

on applicationWillTerminate_(notification)
	-- 关窗/Cmd+Q 统一出口：整树停服（幂等；mlabd 侧 ppid 监视为兜底）。
	logLine("applicationWillTerminate, stopping services")
	if gMlabd is not "" then
		try
			do shell script quoted form of gMlabd & " -stop"
		end try
	end if
end applicationWillTerminate_

on buildMenus()
	set NSMenuClass to current application's NSMenu
	set NSMenuItemClass to current application's NSMenuItem
	set selTerm to current application's NSSelectorFromString("terminate:")
	set selUndo to current application's NSSelectorFromString("undo:")
	set selRedo to current application's NSSelectorFromString("redo:")
	set selCut to current application's NSSelectorFromString("cut:")
	set selCopy to current application's NSSelectorFromString("copy:")
	set selPaste to current application's NSSelectorFromString("paste:")
	set selSelectAll to current application's NSSelectorFromString("selectAll:")
	set selMiniaturize to current application's NSSelectorFromString("performMiniaturize:")
	set selClose to current application's NSSelectorFromString("performClose:")

	set mainMenu to NSMenuClass's alloc()'s init()
	-- App 菜单（第一项；标题由系统按 CFBundleName 显示）
	set appItem to NSMenuItemClass's alloc()'s init()
	set appMenu to NSMenuClass's alloc()'s initWithTitle_("OntoAgents")
	appMenu's addItemWithTitle_action_keyEquivalent_("退出 OntoAgents", selTerm, "q")
	appItem's setSubmenu_(appMenu)
	mainMenu's addItem_(appItem)
	-- 编辑菜单（WKWebView 文本框快捷键依赖）
	set editItem to NSMenuItemClass's alloc()'s init()
	set editMenu to NSMenuClass's alloc()'s initWithTitle_("编辑")
	editMenu's addItemWithTitle_action_keyEquivalent_("撤销", selUndo, "z")
	editMenu's addItemWithTitle_action_keyEquivalent_("重做", selRedo, "Z")
	editMenu's addItemWithTitle_action_keyEquivalent_("剪切", selCut, "x")
	editMenu's addItemWithTitle_action_keyEquivalent_("拷贝", selCopy, "c")
	editMenu's addItemWithTitle_action_keyEquivalent_("粘贴", selPaste, "v")
	editMenu's addItemWithTitle_action_keyEquivalent_("全选", selSelectAll, "a")
	editItem's setSubmenu_(editMenu)
	mainMenu's addItem_(editItem)
	-- 窗口菜单
	set winItem to NSMenuItemClass's alloc()'s init()
	set winMenu to NSMenuClass's alloc()'s initWithTitle_("窗口")
	winMenu's addItemWithTitle_action_keyEquivalent_("最小化", selMiniaturize, "m")
	winMenu's addItemWithTitle_action_keyEquivalent_("关闭", selClose, "w")
	winItem's setSubmenu_(winMenu)
	mainMenu's addItem_(winItem)
	gApp's setMainMenu_(mainMenu)
end buildMenus

on waitBackendReady()
	set urlHealth to envVal("MLAB_HEALTH_URL", (envVal("MLAB_URL", "http://127.0.0.1:8180") & "/healthz"))
	repeat with i from 1 to 40
		try
			do shell script "curl -sf -m 1 " & quoted form of urlHealth
			logLine("backend ready after " & i & " polls")
			return true
		end try
		delay 0.75
	end repeat
	return false
end waitBackendReady

on run
	set gStopping to false
	set gMlabd to envVal("MLAB_MLABD", "")
	set urlStr to envVal("MLAB_URL", "http://127.0.0.1:8180")
	logLine("LAUNCH url=" & urlStr)

	set NSAppClass to current application's NSApplication
	set NSWindowClass to current application's NSWindow
	set WKWVClass to current application's WKWebView
	set WKCfgClass to current application's WKWebViewConfiguration
	set NSURLClass to current application's NSURL
	set NSURLReqClass to current application's NSURLRequest
	set selCenter to current application's NSSelectorFromString("center")
	set selRun to current application's NSSelectorFromString("run")
	set selPerfClose to current application's NSSelectorFromString("performClose:")

	set gApp to NSAppClass's sharedApplication()
	gApp's setActivationPolicy_(0)
	buildMenus()

	if not waitBackendReady() then
		set detail to ""
		try
			set detail to do shell script "cat /tmp/mlab-launcher-last-error.txt 2>/dev/null | tail -8"
		end try
		dialogText("主平台未在 30 秒内就绪（端口 8180）。

" & detail & "

如端口被其他实例或程序占用，请先停止后重试；诊断可从终端运行 .app 内 Resources/bin/mlabd -headless 查看。")
		if gMlabd is not "" then
			try
				do shell script quoted form of gMlabd & " -stop"
			end try
		end if
		error number -12800
	end if

	set w to (envVal("MLAB_WIN_W", "1440")) as integer
	set h to (envVal("MLAB_WIN_H", "900")) as integer
	set rectWin to current application's NSMakeRect(0, 0, w, h)
	set rectWV to current application's NSMakeRect(0, 0, w, h)

	set gWin to NSWindowClass's alloc()'s initWithContentRect_styleMask_backing_defer_(rectWin, 15, 2, false)
	gWin's setTitle_(envVal("MLAB_APP_NAME", "智能体构建平台"))
	gWin's performSelector_(selCenter)
	set cfg to WKCfgClass's alloc()'s init()
	set gWV to WKWVClass's alloc()'s initWithFrame_configuration_(rectWV, cfg)
	gWV's setAutoresizingMask_(18)
	gWin's setContentView_(gWV)
	gWin's setDelegate_(me)
	gApp's setDelegate_(me)
	gWin's makeKeyAndOrderFront_(me)
	gApp's activateIgnoringOtherApps_(true)
	gWV's loadRequest_(NSURLReqClass's requestWithURL_(NSURLClass's URLWithString_(urlStr)))
	logLine("WINDOW-SHOWN entering runloop")
	-- 冒烟自动化可经 MLAB_AUTO_CLOSE_SECONDS 定时自关（窗口生命周期全自助验证）。
	set autoClose to envVal("MLAB_AUTO_CLOSE_SECONDS", "")
	if autoClose is not "" then
		gWin's performSelector_withObject_afterDelay_(selPerfClose, missing value, (autoClose as integer))
	end if
	gApp's performSelector_(selRun)
	logLine("RUN-LOOP-EXITED")
end run
