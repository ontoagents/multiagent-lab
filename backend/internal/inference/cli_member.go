package inference

// REQ-426/M222：外部 CLI 智能体成员执行底座——kind=cli 连接器的成员委派复用既有
// cliAdapter 子进程骨架与 stream-json/纯文本解析层（不另起执行面，claude-code 首件，
// opencode/aider/deepseek-harness 随适配器矩阵）。
// REQ-427/M223：治理拉齐——CLIMemberRunInDocker 沙箱执行变体（docker CLI 直跑，
// 登录态/二进制由镜像自备），stdout 行解析与宿主世界共用同一转译层。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PathProber CLI 适配器可选能力：PATH 文件级探测（exec.LookPath，无 --version 子进程）——
// 装配期轻量检查用（候选可执行名依次尝试，与 Probe 同一口径但不额外 spawn）。
type PathProber interface {
	LookPath() (string, error)
}

// LookPath 实现 cliAdapter 的 PathProber。
func (c *cliAdapter) LookPath() (string, error) {
	for _, b := range c.bin {
		if p, err := exec.LookPath(b); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("PATH 中未找到 %q", c.bin[0])
}

// DockerRunSpec REQ-427②：CLI 成员沙箱执行参数（docker CLI 直跑不引入 SDK，沿 runtime.DockerBackend
// 学习尺度）。镜像须自备目标 CLI 二进制与登录态；Cwd 宿主绑定目录 → 容器 /workspace（安全根）；
// Env 凭据态环境变量（REQ-427③，服务端绑定不进 LLM 上下文）；Hook 过程转译回调（REQ-428）。
type DockerRunSpec struct {
	Bin   string // docker CLI 路径（空 = PATH 查找）
	Image string // 含目标 CLI 的镜像（如 eino-lab/agentd:dev）
	Cwd   string // 宿主绑定目录（空 = 无绑定，容器内继承镜像工作目录）
	Env   map[string]string
	Hook  AuditHook // REQ-428：过程事件回调（nil=静默收集）
}

// AuditHook REQ-428：外部成员过程事件回调（proto_event=text|tool_use|result|a2a_history；
// summary 已截断由调用方定长）——chat 层转译为 external.audit run_event（沿 REQ-206 S3 先例）。
type AuditHook func(protoEvent, summary string)

// memberCollector 成员委派输出收集器（宿主/沙箱两执行世界共用）：
// stream-json 型（claude-code）result 行带 final 标记→取 result 为最终文本（避免 assistant
// 中间文本与 result 重复拼接）；纯文本型拼接全部 delta。
// Hook 非空时逐事件转译过程回调（REQ-428：text/tool_use/result）。
type memberCollector struct {
	text      strings.Builder
	final     string
	toolCalls int
	hook      AuditHook
}

func (m *memberCollector) handle(ev Event) {
	switch ev.Type {
	case "message.delta":
		if v, _ := ev.Data["final"].(bool); v {
			if d, _ := ev.Data["delta"].(string); d != "" {
				m.final = strings.TrimSpace(d)
				if m.hook != nil {
					m.hook("result", strings.TrimSpace(d))
				}
			}
			return
		}
		if d, _ := ev.Data["delta"].(string); d != "" {
			m.text.WriteString(d)
			if m.hook != nil {
				m.hook("text", d)
			}
		}
	case "tool.call":
		m.toolCalls++
		if m.hook != nil {
			name, _ := ev.Data["name"].(string)
			summary := name
			if args, ok := ev.Data["args"].(map[string]any); ok && len(args) > 0 {
				if raw, err := json.Marshal(args); err == nil {
					s := string(raw)
					if len(s) > 160 {
						s = s[:160] + "…"
					}
					summary = name + " " + s
				}
			}
			m.hook("tool_use", summary)
		}
	}
}

func (m *memberCollector) result() string {
	if m.final != "" {
		return m.final
	}
	return strings.TrimSpace(m.text.String())
}

// CLIMemberRun 外部 CLI 成员一次委派执行（宿主世界）：走适配器 Run（同一子进程与解析层），收集文本输出。
// 返回（结果文本, CLI 侧工具调用次数, 错误）。
func CLIMemberRun(ctx context.Context, b Backend, req *RunRequest) (string, int, error) {
	return CLIMemberRunWithAudit(ctx, b, req, nil)
}

// CLIMemberRunWithAudit 同 CLIMemberRun 加过程事件回调（REQ-428：hook 非 null 时逐事件转译）。
func CLIMemberRunWithAudit(ctx context.Context, b Backend, req *RunRequest, hook AuditHook) (string, int, error) {
	ca, ok := b.(*cliAdapter)
	if !ok {
		return "", 0, fmt.Errorf("后端 %q 不是外部 CLI 型（成员通道仅承载 CLI 适配器）", b.Name())
	}
	mc := &memberCollector{hook: hook}
	err := ca.Run(ctx, req, mc.handle)
	if err != nil {
		return "", mc.toolCalls, err
	}
	return mc.result(), mc.toolCalls, nil
}

// memberArgv 适配器面向一次委派的 argv（bin=镜像/宿主内的可执行名，args=适配器构造参数）。
func (c *cliAdapter) memberArgv(req *RunRequest) (string, []string) {
	return c.bin[0], c.buildArgs(req)
}

// CLIMemberRunInDocker 外部 CLI 成员沙箱执行（REQ-427②）：docker run --rm 拉起目标 CLI
//（argv=镜像内 bin+适配器 buildArgs——与宿主世界同一参数构造），stdout 行解析复用适配器
// parseLine（同一转译层）；stderr 缓存用于错误信息；凭据态 Env 经 -e 注入容器。
// 登录态与二进制由镜像自备（宿主 LookPath 不适用于容器世界，装配期不检）。
func CLIMemberRunInDocker(ctx context.Context, b Backend, req *RunRequest, spec DockerRunSpec) (string, int, error) {
	ca, ok := b.(*cliAdapter)
	if !ok {
		return "", 0, fmt.Errorf("后端 %q 不是外部 CLI 型（成员通道仅承载 CLI 适配器）", b.Name())
	}
	dockerBin := spec.Bin
	if dockerBin == "" {
		p, err := exec.LookPath("docker")
		if err != nil {
			return "", 0, fmt.Errorf("docker CLI 不可达: %w", err)
		}
		dockerBin = p
	}
	bin, args := ca.memberArgv(req)
	argv := []string{"run", "--rm"}
	if spec.Cwd != "" {
		argv = append(argv, "-v", spec.Cwd+":/workspace", "-w", "/workspace")
	}
	for k, v := range spec.Env {
		argv = append(argv, "-e", k+"="+v)
	}
	argv = append(argv, spec.Image, bin)
	argv = append(argv, args...)

	cmd := exec.CommandContext(ctx, dockerBin, argv...)
	cmd.Stderr = &bytes.Buffer{}
	cmd.WaitDelay = 3 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("启动沙箱容器: %w", err)
	}
	mc := &memberCollector{hook: spec.Hook}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		ca.parseLine(sc.Text(), func(ev Event) { mc.handle(ev) })
	}
	_ = cmd.Wait()
	if ctx.Err() != nil {
		return "", mc.toolCalls, ctx.Err()
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
		tail := cmd.Stderr.(*bytes.Buffer).String()
		if len(tail) > 800 {
			tail = tail[len(tail)-800:]
		}
		return "", mc.toolCalls, fmt.Errorf("沙箱容器退出码 %d: %s", cmd.ProcessState.ExitCode(), strings.TrimSpace(tail))
	}
	return mc.result(), mc.toolCalls, nil
}
