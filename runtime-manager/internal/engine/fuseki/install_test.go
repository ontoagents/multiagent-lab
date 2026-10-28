package fuseki

// REQ-307 Fuseki 一键安装单测：tar.gz 提取两件（fuseki-server 脚本+jar）/遍历条目拒绝/Install 下载链（httptest 桩）。
// REQ-325 扩 windows zip 形态：zip 提取两件（fuseki-server.bat+jar）/缺件报错/Install 按 URL 扩展名分派提取器
// （windows 不再有 GOOS 拒绝，下载链用例全平台跑）。

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildFakeTarball 构造迷你 apache-jena-fuseki 形态 tar.gz（可选附加额外条目）。
func buildFakeTarball(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake.tar.gz")
	f, _ := os.Create(p)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))})
		_, _ = tw.Write([]byte(content))
	}
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
	return p
}

// buildFakeZip 构造迷你 apache-jena-fuseki 形态 zip（REQ-325 windows 发行包形态）。
func buildFakeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("构造 zip 条目失败: %v", err)
		}
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}

func TestExtractServerFiles(t *testing.T) {
	src := buildFakeTarball(t, map[string]string{
		"apache-jena-fuseki-6.2.0/fuseki-server":     "#!/bin/sh\nexec java -jar fuseki-server.jar",
		"apache-jena-fuseki-6.2.0/fuseki-server.jar": "fake-jar-bytes",
		"apache-jena-fuseki-6.2.0/etc/config.ttl":    "应被跳过",
		"apache-jena-fuseki-6.2.0/bin/s-post":        "应被跳过",
	})
	dest := t.TempDir()
	if err := ExtractServerFiles(src, dest); err != nil {
		t.Fatalf("提取失败: %v", err)
	}
	for _, name := range []string{"fuseki-server", "fuseki-server.jar"} {
		b, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Fatalf("%s 未落盘: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("%s 内容为空", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "config.ttl")); !os.IsNotExist(err) {
		t.Fatalf("非目标文件不应被提取")
	}
}

func TestExtractServerFilesZip(t *testing.T) {
	src := buildFakeZip(t, map[string]string{
		"apache-jena-fuseki-6.2.0/fuseki-server.bat":  "@echo off\njava -jar fuseki-server.jar %*",
		"apache-jena-fuseki-6.2.0/fuseki-server":      "#!/bin/sh\nexec java -jar fuseki-server.jar", // unix 脚本不应被 windows 形态提取
		"apache-jena-fuseki-6.2.0/fuseki-server.jar":  "fake-jar-bytes",
		"apache-jena-fuseki-6.2.0/etc/log4j2.properties": "应被跳过",
	})
	dest := t.TempDir()
	if err := ExtractServerFilesZip(src, dest); err != nil {
		t.Fatalf("zip 提取失败: %v", err)
	}
	for _, name := range []string{"fuseki-server.bat", "fuseki-server.jar"} {
		b, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Fatalf("%s 未落盘: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("%s 内容为空", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "fuseki-server")); !os.IsNotExist(err) {
		t.Fatalf("unix 脚本不应被 zip 形态提取")
	}
	if _, err := os.Stat(filepath.Join(dest, "log4j2.properties")); !os.IsNotExist(err) {
		t.Fatalf("非目标文件不应被提取")
	}
}

func TestExtractRejectsMissingParts(t *testing.T) {
	src := buildFakeTarball(t, map[string]string{
		"x/fuseki-server": "#!/bin/sh", // 缺 jar
	})
	err := ExtractServerFiles(src, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "缺少运行文件") {
		t.Fatalf("缺件应报错，得: %v", err)
	}
}

func TestExtractZipRejectsMissingParts(t *testing.T) {
	src := buildFakeZip(t, map[string]string{
		"x/fuseki-server.bat": "@echo off", // 缺 jar
	})
	err := ExtractServerFilesZip(src, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "缺少运行文件") {
		t.Fatalf("zip 缺件应报错，得: %v", err)
	}
}

// TestInstallDownloadChain 下载→提取链（httptest 桩零外网）。REQ-325 起 windows 无 GOOS 拒绝：
// zip 与 tar.gz 两形态在所有平台同规则（提取器按 URL 扩展名分派）。
func TestInstallDownloadChain(t *testing.T) {
	fakeTgz := buildFakeTarball(t, map[string]string{
		"apache-jena-fuseki-6.2.0/fuseki-server":     "#!/bin/sh\nexit 0",
		"apache-jena-fuseki-6.2.0/fuseki-server.jar": "jar",
	})
	fakeZip := buildFakeZip(t, map[string]string{
		"apache-jena-fuseki-6.2.0/fuseki-server.bat":  "@echo off\nexit 0",
		"apache-jena-fuseki-6.2.0/fuseki-server.jar":  "jar",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			http.ServeFile(w, r, fakeZip)
			return
		}
		http.ServeFile(w, r, fakeTgz)
	}))
	t.Cleanup(srv.Close)
	dl := func(ctx context.Context, url, target string, minBytes int64, _ func(loaded, total int64)) error {
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		f, err := os.Create(target)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, resp.Body)
		return f.Close()
	}
	t.Run("tar.gz", func(t *testing.T) {
		urls := []string{srv.URL + "/jena/binaries/apache-jena-fuseki-" + PinnedVersion + ".tar.gz"}
		dest := t.TempDir()
		if err := Install(context.Background(), dest, urls, dl); err != nil {
			t.Fatalf("Install(tar.gz) 失败: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "fuseki-server.jar")); err != nil {
			t.Fatalf("jar 未落盘: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "fuseki-server")); err != nil {
			t.Fatalf("fuseki-server 脚本未落盘: %v", err)
		}
	})
	t.Run("zip", func(t *testing.T) {
		urls := []string{srv.URL + "/jena/binaries/apache-jena-fuseki-" + PinnedVersion + ".zip"}
		dest := t.TempDir()
		if err := Install(context.Background(), dest, urls, dl); err != nil {
			t.Fatalf("Install(zip) 失败: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "fuseki-server.jar")); err != nil {
			t.Fatalf("jar 未落盘: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "fuseki-server.bat")); err != nil {
			t.Fatalf("fuseki-server.bat 未落盘: %v", err)
		}
	})
}

// TestServeCommandShape REQ-325：windows 直启 java -jar fat jar；unix 走 fuseki-server 脚本。
func TestServeCommandShape(t *testing.T) {
	cmd := serveCommand("data/bin/fuseki-server", "java", "data/bin/fuseki-server.jar", "cfg.ttl", 3030)
	if runtime.GOOS == "windows" {
		if cmd.Path == "" || cmd.Args[0] != "java" || cmd.Args[1] != "-jar" || cmd.Args[2] != "data/bin/fuseki-server.jar" {
			t.Fatalf("windows 应直启 java -jar fat jar，得: %v", cmd.Args)
		}
	} else if cmd.Args[0] != "data/bin/fuseki-server" {
		t.Fatalf("unix 应走 fuseki-server 脚本，得: %v", cmd.Args)
	}
	gotPort := cmd.Args[len(cmd.Args)-1]
	if gotPort != "3030" {
		t.Fatalf("端口参数应为末位 3030，得: %v", cmd.Args)
	}
}

// TestJavaHome REQ-325：java.exe 推导 JAVA_HOME；裸名/不存在路径不猜。
func TestJavaHome(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	javaPath := filepath.Join(binDir, name)
	if err := os.WriteFile(javaPath, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := javaHome(javaPath); got != home {
		t.Fatalf("JAVA_HOME 应推导为 %s，得 %q", home, got)
	}
	if got := javaHome("java"); got != "" {
		t.Fatalf("裸名 java 不应猜 JAVA_HOME，得 %q", got)
	}
	if got := javaHome(filepath.Join(home, "bin", "nope")); got != "" {
		t.Fatalf("不存在路径不应猜 JAVA_HOME，得 %q", got)
	}
}

// TestReasoningConfigWindowsPath REQ-325：写进 Turtle 的路径不得残留反斜杠（非法 Turtle 转义——
// 真机实锤：fuseki config 解析 fatal 致 serve 起不来）。ToSlash 是平台相关的：windows 盘符路径
// 转正斜杠，unix 原生路径（filepath.Join 产物）本就无反斜杠；file URL windows 盘符须三斜杠。
func TestReasoningConfigWindowsPath(t *testing.T) {
	var dbDir string
	if runtime.GOOS == "windows" {
		dbDir = `D:\MultiAgentLab\data\engines\rt_x\tdb` // 盘符绝对路径（filepath.Join("D:",...) 会成盘符相对路径 D:X，勿用）
	} else {
		dbDir = filepath.Join("data", "engines", "rt_x", "tdb")
	}
	cfg := reasoningConfig("ds", dbDir, "", false, true)
	if !strings.Contains(cfg, "tdb:unionDefaultGraph true") {
		t.Fatalf("union 分支应含 unionDefaultGraph true")
	}
	if strings.Contains(cfg, `\`) {
		t.Fatalf("config 不应残留反斜杠:\n%s", cfg)
	}
	if runtime.GOOS == "windows" && !strings.Contains(cfg, `tdb:location "D:/MultiAgentLab/data/engines/rt_x/tdb"`) {
		t.Fatalf("windows 下 tdb:location 应为正斜杠路径，得:\n%s", cfg)
	}
	if runtime.GOOS == "windows" {
		if got := fileURLFromPath(`D:\tmp\owl-fb.rules`); got != "file:///D:/tmp/owl-fb.rules" {
			t.Fatalf("windows file URL 应三斜杠，得 %q", got)
		}
	}
	if got := fileURLFromPath("/tmp/owl-fb.rules"); got != "file:///tmp/owl-fb.rules" {
		t.Fatalf("unix file URL 形态应保持，得 %q", got)
	}
}
