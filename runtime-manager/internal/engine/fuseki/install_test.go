package fuseki

// REQ-307 Fuseki 一键安装单测：tar.gz 提取两件（fuseki-server 脚本+jar）/遍历条目拒绝/Install 下载链（httptest 桩）。

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestExtractRejectsMissingParts(t *testing.T) {
	src := buildFakeTarball(t, map[string]string{
		"x/fuseki-server": "#!/bin/sh", // 缺 jar
	})
	err := ExtractServerFiles(src, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "缺少运行文件") {
		t.Fatalf("缺件应报错，得: %v", err)
	}
}

func TestInstallDownloadChain(t *testing.T) {
	// httptest 桩发行包 → Install 下载+提取链（download 由桩 HTTP 承载）
	fake := buildFakeTarball(t, map[string]string{
		"apache-jena-fuseki-6.2.0/fuseki-server":     "#!/bin/sh\nexit 0",
		"apache-jena-fuseki-6.2.0/fuseki-server.jar": "jar",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, fake)
	}))
	t.Cleanup(srv.Close)
	// 桩发行包 URL（单测零外网依赖）
	urls := []string{srv.URL + "/jena/binaries/apache-jena-fuseki-" + PinnedVersion + ".tar.gz"}
	dest := t.TempDir()
	dl := func(ctx context.Context, url, target string, minBytes int64) error {
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
	if err := Install(context.Background(), dest, urls, dl); err != nil {
		t.Fatalf("Install 失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "fuseki-server.jar")); err != nil {
		t.Fatalf("jar 未落盘: %v", err)
	}
}
