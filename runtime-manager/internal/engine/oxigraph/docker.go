package oxigraph

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine"
)

// ---------------------------------------------------------------------------
// REQ-179/M-O16：oxigraph Docker 执行方式（D-O20 反转 NFR-O-4「子进程为主」口径——
// 有 Docker 时容器执行为默认推荐形态）。
//   装载：数据目录准备与 native 同源（写 load_*.ttl → 容器内 oxigraph load，数据目录挂载）；
//   服务：官方镜像 oxigraph/oxigraph serve --location /data（端口映射宿主 port → 容器 8080）；
//   生命周期：docker rm -f 回收（Process.Stop）；容器名 rt-{profileID} 便于对账与排查。
// Fuseki docker 后置（REQ-379③/M175 评估：社区镜像缺平台灌入链工具链——fuseki 经 manager per-profile 降级 native，docker 化随镜像适配专项轮）。
// ---------------------------------------------------------------------------

const (
	// DefaultImage oxigraph 官方镜像（Docker Hub；可用 OXIGRAPH_DOCKER_IMAGE 覆盖 pin 版本）。
	DefaultImage = "oxigraph/oxigraph"
	// ContainerPrefix 运行容器名前缀（对账与排查锚点）。
	ContainerPrefix = "rt-"
	dockerBinEnv    = "DOCKER_BIN"
)

// DockerImage 执行镜像（env 可覆盖）。
func DockerImage() string {
	if v := strings.TrimSpace(os.Getenv("OXIGRAPH_DOCKER_IMAGE")); v != "" {
		return v
	}
	return DefaultImage
}

// DockerAvailable docker CLI 可用性探测（/api/engines 自检与表单默认值联动用）。
func DockerAvailable() bool {
	bin := dockerBin()
	if bin == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func dockerBin() string {
	if v := strings.TrimSpace(os.Getenv(dockerBinEnv)); v != "" {
		return v
	}
	if p, err := exec.LookPath("docker"); err == nil {
		return p
	}
	return ""
}

func containerName(profileID string) string { return ContainerPrefix + profileID }

// DockerStart 以容器方式装载并启动（准备与 native 同源；load/serve 均经官方镜像）。
// 语义=CompanionMode（legacy 默认图装载、无 union）；regular 方案经 DockerStartOpts（REQ-320）。
func (r *Runtime) DockerStart(ctx context.Context, profileID string, port int, ttls map[string]string) (*engine.Process, error) {
	return r.DockerStartOpts(ctx, profileID, port, ttls, engine.GraphIsolationOpts{CompanionMode: true})
}

// DockerStartOpts 容器方式 per-graph 装载启动（REQ-320/M122）：与 native 同构——
// regular=容器 serve --union-default-graph 起后 GSP 逐本体 PUT（宿主 127.0.0.1 映射端口直达容器
// /store；CLI load 无 --graph 参数同 native）；CompanionMode=容器内 bulk load 原路径（边界不动）。
func (r *Runtime) DockerStartOpts(ctx context.Context, profileID string, port int, ttls map[string]string, opts engine.GraphIsolationOpts) (*engine.Process, error) {
	bin := dockerBin()
	if bin == "" {
		return nil, fmt.Errorf("docker 执行方式不可用：PATH 中未找到 docker CLI")
	}
	dir := filepath.Join(r.DataDir, profileID)
	// 幂等：重建数据目录（与 native 同语义）
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	image := DockerImage()
	run := func(args ...string) (string, error) {
		c := exec.CommandContext(ctx, bin, args...)
		out, err := c.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("docker %s: %v: %s", strings.Join(args, " "), err, tail(out, 300))
		}
		return string(out), nil
	}
	// 容器名清理（重启用同名冲突）
	_, _ = run("rm", "-f", containerName(profileID))
	serveArgs := []string{"run", "-d", "--name", containerName(profileID),
		"-p", fmt.Sprintf("127.0.0.1:%d:8080", port),
		"-v", abs+":/data"}
	if !opts.CompanionMode {
		serveArgs = append(serveArgs, image, "serve", "--location", "/data", "--bind", "0.0.0.0:8080", "--union-default-graph")
		if _, serr := run(serveArgs...); serr != nil {
			return nil, fmt.Errorf("oxigraph 容器启动失败: %s", serr)
		}
		// GSP 逐本体落图（容器刚起未监听由 putGraphRetry 退避吸收）
		for oid, ttl := range ttls {
			if perr := putGraphRetry(ctx, port, engine.GraphIRI(oid), ttl); perr != nil {
				_, _ = run("rm", "-f", containerName(profileID))
				return nil, fmt.Errorf("GSP 落图 %s 失败: %w", oid, perr)
			}
		}
		// REQ-481/M281：附加图装载（inferred 推断图——raw/inferred 双图隔离）
		for grIRI, ttl := range opts.ExtraGraphs {
			if perr := putGraphRetry(ctx, port, grIRI, ttl); perr != nil {
				_, _ = run("rm", "-f", containerName(profileID))
				return nil, fmt.Errorf("GSP 落图 %s 失败: %w", grIRI, perr)
			}
		}
		endpoint := fmt.Sprintf("http://127.0.0.1:%d/query", port)
		return engine.NewProcess(endpoint, 0, func() error {
			c := exec.Command(bin, "rm", "-f", containerName(profileID))
			_, _ = c.CombinedOutput()
			return nil
		}), nil
	}
	// 伴生模式：容器内 oxigraph load（数据目录挂载 /data，与 REQ-309 前行为一致）
	for oid, ttl := range ttls {
		f := filepath.Join(dir, "load_"+sanitizeID(oid)+".ttl")
		if err := os.WriteFile(f, []byte(ttl), 0o644); err != nil {
			return nil, err
		}
		containerFile := "/data/" + filepath.Base(f)
		if _, lerr := run("run", "--rm", "-v", abs+":/data", image,
			"load", "--location", "/data", "--file", containerFile); lerr != nil {
			return nil, fmt.Errorf("容器内装载 %s 失败: %s", oid, lerr)
		}
		_ = os.Remove(f)
	}
	// serve：-p 宿主 port → 容器 8080（REQ-236⑤：端口映射绑 127.0.0.1——容器内 serve 必须
	// 绑 0.0.0.0 才能被宿主网桥转发，但宿主侧只暴露回环，治 0.0.0.0 全网卡暴露可写 /update 端点）
	if _, serr := run("run", "-d", "--name", containerName(profileID),
		"-p", fmt.Sprintf("127.0.0.1:%d:8080", port),
		"-v", abs+":/data",
		image, "serve", "--location", "/data", "--bind", "0.0.0.0:8080"); serr != nil {
		return nil, fmt.Errorf("oxigraph 容器启动失败: %s", serr)
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/query", port)
	return engine.NewProcess(endpoint, 0, func() error {
		c := exec.Command(bin, "rm", "-f", containerName(profileID))
		_, _ = c.CombinedOutput()
		return nil
	}), nil
}

// DockerRunning 容器对账：容器存在且运行中 → 返回 endpoint（服务重启后领用，与 native 孤儿进程对账同思路；
// REQ-236①/M63 起 manager.Reconcile 启动对账正式接线）。
func DockerRunning(profileID string, port int) (string, bool) {
	bin := dockerBin()
	if bin == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "inspect", "-f", "{{.State.Running}}", containerName(profileID)).Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return "", false
	}
	return fmt.Sprintf("http://127.0.0.1:%d/query", port), true
}

// DockerStop 按容器名强制回收（REQ-236①：对账领养句柄的 Stop 闭包）。
func DockerStop(profileID string) error {
	bin := dockerBin()
	if bin == "" {
		return fmt.Errorf("docker CLI 不可用，无法回收容器 %s", containerName(profileID))
	}
	out, err := exec.Command(bin, "rm", "-f", containerName(profileID)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm -f %s: %v: %s", containerName(profileID), err, tail(out, 200))
	}
	return nil
}
