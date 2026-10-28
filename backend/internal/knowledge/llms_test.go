package knowledge

// REQ-372 P3/M265 L3 服务层：llms.txt 聚合视图常跑测试——生成物与落库 llms.md 漂移即缺陷
// （沿 doccheck REQ-399「生成物漂移=测试失败」纪律）。KS_REGEN=1 时改为重生成落盘。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLlmsTxtInSync(t *testing.T) {
	root := "../../../platform-knowledge/09_知识体系"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("知识体系目录不可达（%v）——仓库形态随迁", err)
	}
	got, err := BuildLlmsDoc(root, "platform-knowledge")
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	wantPath := filepath.Join(root, llmsFileName)
	if os.Getenv("KS_REGEN") == "1" {
		if werr := RegenerateLlmsDoc(root, "platform-knowledge"); werr != nil {
			t.Fatalf("重生成失败: %v", werr)
		}
		t.Logf("llms.md 已重生成（%d 字节）", len(got))
		return
	}
	existing, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("llms.md 缺失（%v）——生成物须落库；重生成：KS_REGEN=1 go test ./internal/knowledge -run TestLlmsTxtInSync", err)
	}
	if string(existing) != got {
		// 定位首个差异行，报错信息给到可执行的修复动作
		ol, nl := strings.Split(string(existing), "\n"), strings.Split(got, "\n")
		for i := 0; i < len(ol) || i < len(nl); i++ {
			var a, bb string
			if i < len(ol) {
				a = ol[i]
			}
			if i < len(nl) {
				bb = nl[i]
			}
			if a != bb {
				t.Fatalf("llms.md 与生成器输出漂移（第 %d 行：落库 %q ≠ 生成 %q）——知识卡已变更而聚合未重生成；执行：KS_REGEN=1 go test ./internal/knowledge -run TestLlmsTxtInSync", i+1, a, bb)
			}
		}
		t.Fatal("llms.md 与生成器输出漂移——执行：KS_REGEN=1 go test ./internal/knowledge -run TestLlmsTxtInSync")
	}
}

// TestBuildLlmsDocDeterministic 同输入同字节（生成器确定性契约，与卡目录无关的最小 fixture）。
func TestBuildLlmsDocDeterministic(t *testing.T) {
	root := t.TempDir()
	line := filepath.Join(root, "01_线X", "cards")
	if err := os.MkdirAll(line, 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(name, fm string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(line, name), []byte(fm+"\n正文。\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fmA := "---\nmodule: 知识体系\ntopic: 卡A\ndesc: 第一张\nsynced: 2026-10-28\nkind: concept\ndomain: agent\nprereq: [\"卡B.md\"]\nsources:\n  - REQ-372\ncheck:\n  - q1\n---\n"
	fmB := "---\nmodule: 知识体系\ntopic: 卡B\ndesc: 第二张\nsynced: 2026-10-27\nkind: practice\ndomain: agent\nsources:\n  - REQ-372\ncheck:\n  - q1\n---\n"
	mk("a_卡A.md", fmA)
	mk("b_卡B.md", fmB)
	if err := os.WriteFile(filepath.Join(root, "00_导读.md"), []byte("---\nmodule: 知识体系\ntopic: 导读\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d1, err := BuildLlmsDoc(root, "platform-knowledge")
	if err != nil {
		t.Fatal(err)
	}
	d2, err := BuildLlmsDoc(root, "platform-knowledge")
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatal("同输入两次生成字节不一致（确定性破坏）")
	}
	for _, want := range []string{"共 2 卡：概念 1 · 决策 0 · 实践 1", "synced: 2026-10-28", "01_线X（2 卡）", "[卡A](platform-knowledge/01_线X/cards/a_卡A.md)", "前置 1·源指针 1"} {
		if !strings.Contains(d1, want) {
			t.Errorf("聚合输出缺预期片段: %q", want)
		}
	}
	if strings.Contains(d1, "- [导读]") {
		t.Error("非卡文档（导读）不应入聚合清单")
	}
}
