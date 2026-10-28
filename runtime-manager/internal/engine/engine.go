// Package engine 运行时适配器统一接口（方案 04 §4.2）。
// P1 唯一引擎 oxigraph；fuseki(P2/O6)、memory_graph(P2/O7) 后续按同接口接入。
package engine

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Process 引擎实例句柄（Manager 持有，供健康检查与停止）。
type Process struct {
	Endpoint string // SPARQL 查询端点（POST SPARQL）
	PID      int
	stop     func() error
}

// NewProcess 引擎适配器构造运行句柄。
func NewProcess(endpoint string, pid int, stop func() error) *Process {
	return &Process{Endpoint: endpoint, PID: pid, stop: stop}
}

// Stop 停止引擎子进程。
func (p *Process) Stop() error {
	if p.stop == nil {
		return nil
	}
	return p.stop()
}

// Runtime 引擎适配器接口：Prepare→Start→HealthCheck→Stop。
type Runtime interface {
	// Start 装载本体 TTL 集合并启动服务，返回运行句柄。
	Start(ctx context.Context, profileID string, port int, ttls map[string]string) (*Process, error)
	// HealthCheck 探测引擎可用性（SPARQL ping）。
	HealthCheck(ctx context.Context, endpoint string) error
}

// ReasoningRuntime 支持推理开关的引擎（O6：fuseki；config.reasoning=true 时启用 RDFS/OWL 推理）。
type ReasoningRuntime interface {
	Runtime
	StartWithReasoning(ctx context.Context, profileID string, port int, ttls map[string]string, reasoning bool) (*Process, error)
}

// GraphIsolationOpts per-graph 装载语义开关（REQ-320/M122）。
type GraphIsolationOpts struct {
	// CompanionMode 伴生宿主方案（方案名「伴生·」前缀，REQ-309 池与存量伴生宿主）：
	// 保持 legacy 默认图装载路径与对账口径、不开 union——伴生读写走自描述 GRAPH 块
	// 与 GRAPH ont-{id}，池「子图互不可见」验收不可破（REQ-320 P1 边界）。
	CompanionMode bool
	// Reasoning 推理开关（fuseki O6；GraphIsolation 路由时自 profile config 透传，
	// 防止 per-graph 化丢失推理装配）。
	Reasoning bool
	// ExtraGraphs 附加命名图（REQ-481/M281：graphIRI→TTL——装载期并置物化的推断三元组
	// 装独立 inferred 图，raw/inferred 双图隔离 provenance 可分）。oxigraph per-graph 路径
	// 装载；fuseki 忽略（InfModel 查询期推理形态维持不动）；伴生模式忽略。
	ExtraGraphs map[string]string
}

// GraphIsolationRuntime 支持 per-graph 装载与 union 默认图开关的引擎（REQ-320；可选实现）。
// regular 方案=逐本体独立 named graph（GraphIRI）+ union 默认图（消费面零改动）；
// 伴生方案=legacy 路径原样（CompanionMode）。
type GraphIsolationRuntime interface {
	Runtime
	StartWithGraphIsolation(ctx context.Context, profileID string, port int, ttls map[string]string, opts GraphIsolationOpts) (*Process, error)
}

// GraphIRI 本体在方案引擎内的独立命名图（REQ-320①：urn:onto:{id}，与伴生 GRAPH ont-{id}
// 隔离同构；装载与对账两侧共用本函数派生，保证图名一致）。
func GraphIRI(ontologyID string) string {
	var b strings.Builder
	b.WriteString("urn:onto:")
	for _, r := range ontologyID {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// EngineStatus 引擎可执行文件自检结果（REQ-146）。
type EngineStatus struct {
	Engine           string   `json:"engine"`
	Registered       bool     `json:"registered"`                   // runtimed 启动时是否注册了适配器
	Installed        bool     `json:"installed"`                    // 本地是否探测到可执行文件
	Installing       bool     `json:"installing,omitempty"`         // 一键安装进行中
	LastInstallError string   `json:"last_install_error,omitempty"` // 最近一次一键安装失败原因
	Binary           string   `json:"binary,omitempty"`             // 命中的可执行文件路径
	Version          string   `json:"version,omitempty"`            // 引擎版本（best-effort）
	Searched         []string `json:"searched,omitempty"`           // 探测过的候选路径
	Hint             string   `json:"hint,omitempty"`               // 缺失时的安装指引
	Installable      bool     `json:"installable"`                  // 是否支持一键下载安装
	InstallProgress  *float64 `json:"install_progress,omitempty"`   // 一键安装下载进度 0~100（REQ-471；仅本引擎安装中且总量已知时透出）
}

// StatusProbe 引擎自检能力（适配器可选实现；未实现按未注册呈现）。
type StatusProbe interface {
	Probe() EngineStatus
}

// FindExecutable 按候选序找可执行文件（REQ-146 共享助手）：
// 含路径分隔符的候选做 Stat + 可执行检查；裸名走 PATH 查找。返回命中的路径。
func FindExecutable(paths []string) (string, bool) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if strings.ContainsRune(p, '/') || strings.ContainsRune(p, '\\') {
			if st, err := os.Stat(p); err == nil && isExecutableFile(st) {
				return p, true
			}
			continue
		}
		if lp, err := exec.LookPath(p); err == nil {
			return lp, true
		}
	}
	return "", false
}

// isExecutableFile Windows 移植（REQ-321）：Go 的 os.Stat 在 Windows 对 .exe 不置执行位
// （实测 Mode()=-rw-rw-rw-），原 Mode()&0o111 检查在 Windows 恒假致引擎恒判「未安装」；
// Windows 侧以「存在且非目录」为判据（候选序均为显式配置路径/已知二进制名，误配由执行时报错兜底）。
// 目录恒不命中（unix x 位语义保持——0700 目录带执行位但非文件）。
func isExecutableFile(st os.FileInfo) bool {
	if st.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return st.Mode()&0o111 != 0
}
