package store

import (
	"path/filepath"
	"testing"
	"time"
)

// REQ-310/M114：last_error_at 发生时间戳语义（对 origin/main 实现的回归锁定）——
// 写错误落时刻（SQLite CURRENT_TIMESTAMP，UTC）、重复失败覆盖为新发生时刻、
// 成功收敛（lastErr=""）清空；错误状态无自动老化（「最后失败记忆」仅成功启动收敛）。
func TestSetStatusLastErrorAt(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "rt.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	p := &Profile{ID: "p1", Name: "n", Engine: "oxigraph", OntologyIDs: []string{"o1"}, Config: "{}", Port: 9201}
	if err := st.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	// 无错误：last_error_at 恒空
	if err := st.SetStatus(p.ID, "running", "", ""); err != nil {
		t.Fatalf("set running: %v", err)
	}
	got, _ := st.Get(p.ID)
	if got.LastErrorAt != "" {
		t.Fatalf("no-error status should not set last_error_at, got %q", got.LastErrorAt)
	}

	// 写错误：落发生时刻（CURRENT_TIMESTAMP = "YYYY-MM-DD HH:MM:SS" UTC，不早于测试窗口起点）
	before := time.Now().UTC().Add(-time.Minute)
	if err := st.SetStatus(p.ID, "error", "拉取本体 o1 失败: 404 Not Found", ""); err != nil {
		t.Fatalf("set error: %v", err)
	}
	got, _ = st.Get(p.ID)
	if got.LastErrorAt == "" {
		t.Fatalf("error status must record last_error_at")
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", got.LastErrorAt, time.UTC)
	if err != nil {
		t.Fatalf("last_error_at not SQLite datetime: %v (%q)", err, got.LastErrorAt)
	}
	if ts.Before(before) {
		t.Fatalf("last_error_at %v earlier than test window", ts)
	}

	// 重复失败：覆盖为新发生时刻（冒烟回放等触发的刷新不保留旧时刻）
	time.Sleep(1100 * time.Millisecond) // CURRENT_TIMESTAMP 秒级精度，跨秒断言刷新
	if err := st.SetStatus(p.ID, "error", "again", ""); err != nil {
		t.Fatalf("set error 2: %v", err)
	}
	got2, _ := st.Get(p.ID)
	if got2.LastErrorAt <= got.LastErrorAt {
		t.Fatalf("repeated failure should refresh last_error_at: %q <= %q", got2.LastErrorAt, got.LastErrorAt)
	}

	// 成功收敛：错误与时刻一并清空（CASE WHEN ?='' THEN NULL；IFNULL 读回空串）
	if err := st.SetStatus(p.ID, "running", "", ""); err != nil {
		t.Fatalf("recover: %v", err)
	}
	got3, _ := st.Get(p.ID)
	if got3.LastError != "" || got3.LastErrorAt != "" {
		t.Fatalf("recovery should clear last_error(_at), got %q / %q", got3.LastError, got3.LastErrorAt)
	}
}
