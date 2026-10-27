// REQ-450：外部推理后端安装引导。
//
// 定位=引导安装而非代为安装：平台不下载不托管（安装器异构——npm/curl/pip 各异，
// 长时下载不进创建主链；对照 REQ-146——oxigraph 一键安装仅单一 release 资产成立，
// fuseki 同 REQ 仅手动指引）。dsh 系无公开安装命令，只给凭据/获取说明，不臆造命令。
package inference

// InstallHint 后端安装引导（随 Status 对外透出，前端「安装指引」Popover 消费）。
type InstallHint struct {
	Cmd  string // 安装命令（无公开安装命令的后端留空，以 Note 说明）
	Docs string // 官方安装文档 URL
	Note string // 补充说明（获取渠道/凭据配置等）
}

// installHints 按后端名索引；eino-adk 为平台内置默认，恒无引导。
var installHints = map[string]InstallHint{
	"claude-code": {Cmd: "npm install -g @anthropic-ai/claude-code", Docs: "https://docs.anthropic.com/en/docs/claude-code/overview"},
	"opencode":    {Cmd: "curl -fsSL https://opencode.ai/install | bash", Docs: "https://opencode.ai/docs"},
	"aider":       {Cmd: "python -m pip install aider-install && aider-install", Docs: "https://aider.chat/docs/install.html"},
	"deepseek-harness": {
		Note: "dsh CLI 需自备并加入 PATH（外部 harness，平台不代装）；凭据经 dsh web Models 页或 DEEPSEEK_API_KEY 一次性配置",
	},
	"deepseek-harness-acp": {
		Note: "同 deepseek-harness：dsh CLI 自备加入 PATH；凭据经 dsh web Models 页或 DEEPSEEK_API_KEY 配置",
	},
	"deepseek-harness-sdk": {
		Note: "同 deepseek-harness：dsh CLI 自备加入 PATH；凭据经 dsh web Models 页或 DEEPSEEK_API_KEY 配置",
	},
}
