package fuseki

// REQ-329 回归单测：fuseki 推理启动链的相对路径解析。
// 真机实锤（WSL，FUSEKI_BIN 未配置）：resolveBinary 兜底候选 data/bin/fuseki-server 为相对路径，
// extractRulesFile 以 jarCmd.Dir 换子进程 cwd 后，相对 jar 路径在子进程 cwd 下解析失败——
// jar 工具报 NoSuchFileException: data/bin/fuseki-server.jar（exit 1，栈帧 ZipFile.<init> 经
// tail(200) 截剩误导性片段，表象酷似 jar 损坏）。

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildFakeServerJar 构造含 etc/owl-fb.rules 条目的真实 zip 形态 jar（jar xf 可解）。
func buildFakeServerJar(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("etc/owl-fb.rules")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("# fake owl-fb rules")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestExtractRulesFileRelativePathResolution 复刻真机缺陷形态：cwd=仓库根、jar 与目标目录均以
// 相对路径传入（候选序兜底候选 + 相对 ENGINE_DATA_DIR 的产物）——解压仍须成功，且返回绝对路径
// （返回值经 fileURLFromPath 进 config.ttl，相对路径会拼出锚在文件系统根的错误 file: URL）。
// 修复前本用例在 jar 工具侧 NoSuchFileException 失败。
func TestExtractRulesFileRelativePathResolution(t *testing.T) {
	if _, err := exec.LookPath("jar"); err != nil {
		t.Skip("PATH 上无 jar 工具（JDK 未装），跳过")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "data", "bin")
	destDir := filepath.Join(root, "data", "engines", "rt_x")
	for _, d := range []string{binDir, destDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	buildFakeServerJar(t, filepath.Join(binDir, "fuseki-server.jar"))
	t.Chdir(root) // 模拟 runtimed 从仓库根启动、候选序命中相对 data/bin

	dst, err := extractRulesFile("java", "data/bin", "data/engines/rt_x")
	if err != nil {
		t.Fatalf("相对路径入参应解压成功: %v", err)
	}
	if !filepath.IsAbs(dst) {
		t.Fatalf("返回路径应为绝对路径（file: URL 依赖），得 %q", dst)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("规则文件未落盘: %v", err)
	}
	if !strings.Contains(string(b), "fake owl-fb rules") {
		t.Fatalf("规则文件内容不符: %q", b)
	}
	// 缺陷形态下 NoSuchFile 不落任何文件；防御性断言子进程 cwd 换向不会产生错位产物
	if _, err := os.Stat(filepath.Join(destDir, "data")); !os.IsNotExist(err) {
		t.Fatalf("相对路径不应在 destDir 下再生 data/ 层级")
	}
}

// TestExtractRulesFileReuse 已解出的规则文件直接复用（同 profile 重启不重复解压）。
func TestExtractRulesFileReuse(t *testing.T) {
	root := t.TempDir()
	destDir := filepath.Join(root, "data", "engines", "rt_x")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := os.WriteFile(filepath.Join(destDir, "owl-fb.rules"), []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	// jar 不存在也应命中复用分支返回（不触发 jar 工具）
	dst, err := extractRulesFile("java", filepath.Join(root, "data", "bin"), "data/engines/rt_x")
	if err != nil {
		t.Fatalf("已解出应复用: %v", err)
	}
	if want := filepath.Join(destDir, "owl-fb.rules"); dst != want {
		t.Fatalf("应返回 %q，得 %q", want, dst)
	}
}
