; 智能体构建平台 Windows 桌面安装包（REQ-321/M123）
; 编译：ISCC installer.iss（或运行 build.ps1）；产物 dist\OntoAgents_<版本>_setup.exe
; 形态：per-user 免管理员安装（默认 %LOCALAPPDATA%\Programs\OntoAgents）；
; 卸载保留 data\（平台数据）；随包分发三服务 + oxigraph + 前端产物 + 平台知识/docs。
; 语言：优先中文（build\ChineseSimplified.isl 由 build.ps1 缓存；缺失回退英文 Default.isl）。

#define MyAppName "智能体构建平台"
#define MyAppVersion "1.0.0"
#define MyAppExe "OntoAgents.exe"
#define Staging "build\staging"

[Setup]
AppId={{7E2F5C86-4B9A-4C31-9E8D-2A5F1B6C0D43}}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=ontoagents
DefaultDirName={localappdata}\Programs\OntoAgents
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
OutputDir=dist
OutputBaseFilename=OntoAgents_{#MyAppVersion}_setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\{#MyAppExe}
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

#if FileExists("build\ChineseSimplified.isl")
[Languages]
Name: "chinesesimplified"; MessagesFile: "build\ChineseSimplified.isl"
#else
[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
#endif

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#Staging}\*"; DestDir: "{app}"; Flags: recursesubdirs createallsubdirs ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExe}"; Comment: "启动 {#MyAppName}（自动拉起本地服务）"
Name: "{group}\停止平台服务"; Filename: "{app}\{#MyAppExe}"; Parameters: "-stop"; Comment: "停止智能体构建平台后台服务"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExe}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExe}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent

[Code]
// 卸载前回收平台服务进程（安装目录内固定 exe 名；数据目录 data\ 有意保留，见 README.txt）
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  ResultCode: Integer;
begin
  if CurUninstallStep = usUninstall then
    Exec(ExpandConstant('{cmd}'),
      '/C taskkill /F /T /IM OntoAgents.exe & taskkill /F /T /IM backend.exe & taskkill /F /T /IM ontologyd.exe & taskkill /F /T /IM runtimed.exe',
      '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;
