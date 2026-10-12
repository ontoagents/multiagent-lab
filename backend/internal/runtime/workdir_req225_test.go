// REQ-225①/M53 单测：docker Start 的 work_dir 同路径 volume 绑定与沙箱执行世界标记
//（stub docker CLI 记录 run 参数；零外部依赖）。
package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubDockerBin 记录 run 参数的 docker CLI 桩（volume 绑定断言用）。
func stubDockerBin(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	bin := filepath.Join(dir, "docker")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\ncase \"$*\" in\n  *version*) echo 24.0.0 ;;\n  *inspect*) echo exited ;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func TestDockerStartWorkdirVolumeBinding(t *testing.T) {
	bin, log := stubDockerBin(t)
	d := &DockerBackend{Bin: bin, Image: "agentd:dev", PlatformURL: "http://host:8080", TokenIssue: func(string) (string, error) { return "tok", nil }, HealthzWait: 2 * time.Second}
	wd := t.TempDir()
	// stub 无真实 agentd healthz——Start 以 not healthy 收尾属预期；断言点是 run 参数日志
	_, _ = d.Start(context.Background(), StartSpec{AgentID: "ag1", WorkDir: wd})
	b, _ := os.ReadFile(log)
	args := string(b)
	if !strings.Contains(args, "-v "+wd+":"+wd) {
		t.Fatalf("work_dir 应同路径 volume 绑定: %s", args)
	}
	if !strings.Contains(args, "SANDBOX_EXEC_WORLD=1") {
		t.Fatalf("沙箱执行世界标记缺失: %s", args)
	}
	// 相对路径不绑定
	bindings := strings.Count(args, "-v")
	_, _ = d.Start(context.Background(), StartSpec{AgentID: "ag2", WorkDir: "relative/path"})
	b2, _ := os.ReadFile(log)
	if strings.Count(string(b2), "-v") != bindings { // ag2 的 run 不带 -v（-v 总数不变）
		t.Fatalf("相对路径 work_dir 不应绑定: %s", string(b2))
	}
}
