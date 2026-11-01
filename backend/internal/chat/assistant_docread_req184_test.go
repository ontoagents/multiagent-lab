package chat

// REQ-184 收尾（2026-10-22）：doc_read 工具 research/ 分支退役行为测试——旧路径返回诚实指引
// （模型可自纠）；REQ-570 四区收敛（2026-11-01）：platform-knowledge/ 分支同规则退役，
// docs/（handbook 现状手册等三子区）照常可读。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
)

func TestDocReadToolResearchRetired(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs", "handbook") // 现状手册区（docs/ 前缀下三子区之一）
	knowledge := filepath.Join(root, "docs", "learning")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(knowledge, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "02_智能体_技术方案设计.md"), []byte("# 02"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(knowledge, "00_智能体模块导读.md"), []byte("# 智能体导读"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := AssistantDeps{DocsRoot: filepath.Join(root, "docs"), KnowledgeRoot: docs}

	// 工具描述不再提及 research/
	entry := assistantDocRead(deps)
	if strings.Contains(entry.Description, "research/") {
		t.Fatalf("doc_read 描述应不再含 research/: %s", entry.Description)
	}

	bt, err := entry.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	invokable, ok := bt.(einotool.InvokableTool)
	if !ok {
		t.Fatalf("doc_read 应为 InvokableTool: %T", bt)
	}

	// research/ 旧路径：诚实指引
	_, err = invokable.InvokableRun(context.Background(), `{"path":"research/old_survey.md"}`)
	if err == nil || !strings.Contains(err.Error(), "research/ 目录已退役") || !strings.Contains(err.Error(), "docs/research/") {
		t.Fatalf("research/ 旧路径应返回诚实指引: %v", err)
	}

	// platform-knowledge/ 旧路径（REQ-570 退役）：诚实指引
	_, err = invokable.InvokableRun(context.Background(), `{"path":"platform-knowledge/00_智能体模块导读.md"}`)
	if err == nil || !strings.Contains(err.Error(), "platform-knowledge/ 目录已退役") || !strings.Contains(err.Error(), "docs/handbook") {
		t.Fatalf("platform-knowledge/ 旧路径应返回诚实指引: %v", err)
	}

	// docs/ 照常可读（handbook 子区）
	out, err := invokable.InvokableRun(context.Background(), `{"path":"docs/handbook/02_智能体_技术方案设计.md"}`)
	if err != nil || !strings.Contains(out, "# 02") {
		t.Fatalf("docs/handbook/ 应照常可读: %v %s", err, out)
	}

	// docs/learning 子区照常可读
	out, err = invokable.InvokableRun(context.Background(), `{"path":"docs/learning/00_智能体模块导读.md"}`)
	if err != nil || !strings.Contains(out, "# 智能体导读") {
		t.Fatalf("docs/learning/ 应照常可读: %v %s", err, out)
	}
}
