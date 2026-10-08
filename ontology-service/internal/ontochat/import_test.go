package ontochat

import (
	"strings"
	"testing"
)

// REQ-277：快速模式首轮导入——Description 以文件名/首段补齐并推进 domain；材料进 Hints。
func TestImportFileFirstTurn(t *testing.T) {
	st := newJobTestStore(t)
	if _, err := st.Create("sess-imp", "导入测试"); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.Get("sess-imp") // stage=cq
	e := &Engine{}
	res, err := e.ImportFile(st, sess, "领域调研.md", "# 测序数据质控\nFASTQ 质控指标包括 Q30、覆盖率等。")
	if err != nil {
		t.Fatal(err)
	}
	if res.NextStage != "domain" {
		t.Fatalf("首轮导入应推进 domain: %s", res.NextStage)
	}
	back, _ := st.Get("sess-imp")
	if back.Context.Description == "" {
		t.Fatal("首轮导入应补齐 Description")
	}
	if len(back.Context.Hints) != 1 || !strings.Contains(back.Context.Hints[0], "[文件导入 领域调研.md]") {
		t.Fatalf("材料应带来源前缀进 Hints: %+v", back.Context.Hints)
	}
	if !strings.Contains(res.Reply, "已导入文件材料") {
		t.Fatalf("回复形态不符: %.60s", res.Reply)
	}
}

// REQ-277：domain 阶段导入=追加材料；超长截断如实标注；story 阶段禁用。
func TestImportFileGuards(t *testing.T) {
	st := newJobTestStore(t)
	_, _ = st.Create("sess-imp2", "导入测试2")
	sess, _ := st.Get("sess-imp2")
	e := &Engine{}
	// domain 追加
	sess.Stage = "domain"
	sess.Context.Description = "已有描述"
	if _, err := e.ImportFile(st, sess, "a.yaml", "key: value"); err != nil {
		t.Fatal(err)
	}
	back, _ := st.Get("sess-imp2")
	if back.Context.Description != "已有描述" {
		t.Fatal("domain 导入不应改 Description")
	}
	// 超长截断
	big := strings.Repeat("长", ImportFileMaxChars+500)
	res, _ := e.ImportFile(st, sess, "big.txt", big)
	if !strings.Contains(res.Reply, "截断至 30000") {
		t.Fatalf("超长应标注截断: %.120s", res.Reply)
	}
	hint := sess.Context.Hints[len(sess.Context.Hints)-1]
	if len([]rune(hint)) > ImportFileMaxChars+50 {
		t.Fatalf("材料应截断: %d", len([]rune(hint)))
	}
	// story 禁用
	sess.Stage = "story"
	if _, err := e.ImportFile(st, sess, "x.md", "内容"); err == nil {
		t.Fatal("story 阶段应禁用导入")
	}
	// 空内容
	sess.Stage = "domain"
	if _, err := e.ImportFile(st, sess, "empty.txt", "  "); err == nil {
		t.Fatal("空内容应报错")
	}
}
