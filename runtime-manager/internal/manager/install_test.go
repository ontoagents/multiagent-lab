package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// REQ-146 下载机制零网络单测（httptest 模拟 release 服务器）。

func TestDownloadToSuccess(t *testing.T) {
	// 2MB 合法内容（超过 1MB 下限）
	payload := strings.Repeat("oxigraph-bin-payload-", 100<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "oxigraph")
	if err := downloadTo(context.Background(), srv.URL, target, 1<<20, nil); err != nil {
		t.Fatalf("downloadTo: %v", err)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("target missing: %v", err)
	}
	if st.Size() != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", st.Size(), len(payload))
	}
	if runtime.GOOS != "windows" && st.Mode()&0o111 == 0 {
		// Windows 无执行位语义（REQ-321）：chmod 在 Windows 为空操作，跳过断言
		t.Fatal("执行位未设置")
	}
	if _, err := os.Stat(target + ".download"); !os.IsNotExist(err) {
		t.Fatal("temp 文件应已原子替换移除")
	}
}

func TestDownloadToFailures(t *testing.T) {
	// 404：不落盘
	srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv404.Close()
	dir := t.TempDir()
	target := filepath.Join(dir, "oxigraph")
	if err := downloadTo(context.Background(), srv404.URL, target, 1, nil); err == nil {
		t.Fatal("404 应报错")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("404 不应落盘")
	}

	// 内容过短（低于 minBytes）：防错误页/截断落盘
	srvShort := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("short"))
	}))
	defer srvShort.Close()
	if err := downloadTo(context.Background(), srvShort.URL, target, 1<<20, nil); err == nil {
		t.Fatal("短内容应报错")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("短内容不应落盘")
	}

	// 服务端中断流：报错且清理 temp
	srvCut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		_, _ = w.Write([]byte(strings.Repeat("x", 1024)))
		// 提前断开
		panic(http.ErrAbortHandler)
	}))
	defer srvCut.Close()
	if err := downloadTo(context.Background(), srvCut.URL, target, 1<<20, nil); err == nil {
		t.Fatal("中断流应报错")
	}
	if _, err := os.Stat(target + ".download"); !os.IsNotExist(err) {
		t.Fatal("中断后 temp 应清理")
	}
}

// TestDownloadToProgress REQ-471：Content-Length 已知时按下载推进回调（终态 loaded==total）；
// 无 Content-Length 时不回调。零网络单测（httptest）。
func TestDownloadToProgress(t *testing.T) {
	payload := strings.Repeat("progress-payload-", 8<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 大于 2KB 写缓冲的单发 Write 会被 net/http 改走 chunked——镜像源（Apache/GitHub）实际
		// 恒带 Content-Length，此处显式置上模拟真实形态
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	var calls int
	var lastLoaded, lastTotal int64
	err := downloadTo(context.Background(), srv.URL, filepath.Join(t.TempDir(), "oxigraph"), 1,
		func(loaded, total int64) {
			calls++
			lastLoaded, lastTotal = loaded, total
		})
	if err != nil {
		t.Fatalf("downloadTo: %v", err)
	}
	if calls == 0 || lastTotal != int64(len(payload)) || lastLoaded != lastTotal {
		t.Fatalf("进度回调异常: calls=%d last=%d/%d want 终态 %d/%d", calls, lastLoaded, lastTotal, len(payload), len(payload))
	}

	// chunked（服务器不透出总量）：不回调，下载仍成功
	srvChunked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 4; i++ {
			_, _ = w.Write([]byte(payload))
			if f, ok := w.(http.Flusher); ok {
				f.Flush() // 强制 chunked，ContentLength=-1
			}
		}
	}))
	defer srvChunked.Close()
	calls = 0
	if err := downloadTo(context.Background(), srvChunked.URL, filepath.Join(t.TempDir(), "oxigraph"), 1,
		func(loaded, total int64) { calls++ }); err != nil {
		t.Fatalf("无总量 downloadTo: %v", err)
	}
	if calls != 0 {
		t.Fatalf("无 Content-Length 不应回调，得 %d 次", calls)
	}
}
