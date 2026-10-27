package chat

// REQ-451 平台知识阅读补全单测：
// ①doc_read 分段语义（默认全文/48KB 截断附续读偏移/offset_line+max_lines 分页与续读提示/越界报错）
// ②doc_list 目录发现（路径清单+frontmatter topic/desc 摘要+prefix 过滤+非 md 排除）
// ③doc_grep 词法定位（file:line:text 命中/正则非法降级字面量/prefix 限定/无命中话术）
// ④注册表归位（新工具注册齐 + 五件平台知识工具 Group 标注）。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

func newDocFixture(t *testing.T) AssistantDeps {
	t.Helper()
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	knowledge := filepath.Join(root, "platform-knowledge")
	for _, d := range []string{docs, filepath.Join(knowledge, "02_智能体"), filepath.Join(knowledge, "04_本体")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(docs, "01_智能体_需求文档_PRD.md"): "# PRD\n关键词 needle 在这里\n第三行",
		filepath.Join(knowledge, "02_智能体", "00_智能体模块.md"): "---\nmodule: 智能体\ntopic: 智能体模块导读\ndesc: 智能体域总览与文档地图\nsynced: 2026-10-27\n---\n# 智能体\nneedle 精读入口",
		filepath.Join(knowledge, "04_本体", "90_底层实现原理.md"): "# 本体原理\n无关键词",
		filepath.Join(knowledge, "04_本体", "notes.txt"): "txt 不应出现",
		filepath.Join(docs, "00_特殊字符.md"):             "占比100%示例行\n不含关键词",
	}
	for p, c := range files {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return AssistantDeps{DocsRoot: docs, KnowledgeRoot: knowledge}
}

func invoke(t *testing.T, e *tool.Entry, args string) (string, error) {
	t.Helper()
	bt, err := e.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := bt.(einotool.InvokableTool)
	if !ok {
		t.Fatalf("%s 应为 InvokableTool: %T", e.ID, bt)
	}
	return inv.InvokableRun(context.Background(), args)
}

func TestDocReadPaged(t *testing.T) {
	deps := newDocFixture(t)
	e := assistantDocRead(deps)

	// 默认全文（无 offset）：旧行为保留
	out, err := invoke(t, e, `{"path":"platform-knowledge/02_智能体/00_智能体模块.md"}`)
	if err != nil || !strings.Contains(out, "needle 精读入口") || strings.Contains(out, "第 1~") {
		t.Fatalf("默认应全文且无分页头: %v %s", err, out)
	}

	// 分段读：末页无续读提示（夹具 docs/01 共 3 行）
	out, err = invoke(t, e, `{"path":"docs/01_智能体_需求文档_PRD.md","offset_line":3,"max_lines":2}`)
	if err != nil || !strings.Contains(out, "第 3~3 行 / 共 3 行") || !strings.Contains(out, "第三行") {
		t.Fatalf("分段读末页形态不符: %v %s", err, out)
	}

	// 分段读：有后续页附续读偏移
	out, err = invoke(t, e, `{"path":"docs/01_智能体_需求文档_PRD.md","offset_line":1,"max_lines":2}`)
	if err != nil || !strings.Contains(out, "第 1~2 行 / 共 3 行") || !strings.Contains(out, "offset_line=3") {
		t.Fatalf("分段读续读提示缺失: %v %s", err, out)
	}

	// 越界偏移如实报错（模型可自纠）
	if _, err = invoke(t, e, `{"path":"docs/01_智能体_需求文档_PRD.md","offset_line":99}`); err == nil || !strings.Contains(err.Error(), "超出文档行数") {
		t.Fatalf("越界 offset 应报错: %v", err)
	}

	// 全文超 48KB 截断：附总行数与续读偏移
	big := filepath.Join(deps.DocsRoot, "02_大档.md")
	var sb strings.Builder
	for i := 1; i <= 3000; i++ {
		fmt.Fprintf(&sb, "行%d：平台文档分段阅读验证内容填充\n", i)
	}
	if err := os.WriteFile(big, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = invoke(t, e, `{"path":"docs/02_大档.md"}`)
	if err != nil || !strings.Contains(out, "已截断") || !strings.Contains(out, "共 3000 行") || !strings.Contains(out, "offset_line=") {
		t.Fatalf("截断应附总行数与续读偏移: %v %s", err, out)
	}
}

func TestDocListTool(t *testing.T) {
	deps := newDocFixture(t)
	e := assistantDocList(deps)
	if e.Group != ToolGroupPlatformKnowledge {
		t.Fatalf("doc_list 应标注平台知识分组: %q", e.Group)
	}

	out, err := invoke(t, e, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"docs/01_智能体_需求文档_PRD.md", "platform-knowledge/02_智能体/00_智能体模块.md", "智能体模块导读", "智能体域总览", "共 4 篇"} {
		if !strings.Contains(out, want) {
			t.Fatalf("目录清单缺 %q：%s", want, out)
		}
	}
	if strings.Contains(out, "notes.txt") {
		t.Fatalf("非 md 应排除：%s", out)
	}

	// prefix 过滤
	out, err = invoke(t, e, `{"prefix":"platform-knowledge/04_本体"}`)
	if err != nil || !strings.Contains(out, "90_底层实现原理.md") || strings.Contains(out, "00_智能体模块.md") {
		t.Fatalf("prefix 过滤不符: %v %s", err, out)
	}

	// 无匹配话术
	out, err = invoke(t, e, `{"prefix":"platform-knowledge/99_不存在"}`)
	if err != nil || !strings.Contains(out, "无匹配文档") {
		t.Fatalf("无匹配话术不符: %v %s", err, out)
	}
}

func TestDocGrepTool(t *testing.T) {
	deps := newDocFixture(t)
	e := assistantDocGrep(deps)
	if e.Group != ToolGroupPlatformKnowledge {
		t.Fatalf("doc_grep 应标注平台知识分组: %q", e.Group)
	}

	// 字面量命中（file:line:text，路径可直喂 doc_read）
	out, err := invoke(t, e, `{"pattern":"needle"}`)
	if err != nil || !strings.Contains(out, "docs/01_智能体_需求文档_PRD.md:2:") || !strings.Contains(out, "platform-knowledge/02_智能体/00_智能体模块.md:8:") {
		t.Fatalf("词法命中形态不符: %v %s", err, out)
	}
	_ = out

	// 正则非法降级字面量（沿 tool/grep 先例）——"100%" 非法正则，按字面量仍命中
	out, err = invoke(t, e, `{"pattern":"100%"}`)
	if err != nil || !strings.Contains(out, "docs/00_特殊字符.md:1:") {
		t.Fatalf("正则非法应降级字面量: %v %s", err, out)
	}

	// prefix 限定
	out, err = invoke(t, e, `{"pattern":"needle","prefix":"docs/"}`)
	if err != nil || !strings.Contains(out, "命中 1 条") || strings.Contains(out, "platform-knowledge/") {
		t.Fatalf("prefix 限定不符: %v %s", err, out)
	}

	// 无命中话术
	out, err = invoke(t, e, `{"pattern":"不存在词xyz"}`)
	if err != nil || !strings.Contains(out, "无命中") {
		t.Fatalf("无命中话术不符: %v %s", err, out)
	}
}

func TestAssistantToolsRegistryReq451(t *testing.T) {
	deps := newDocFixture(t)
	reg := tool.NewRegistry()
	if err := RegisterAssistantTools(reg, deps); err != nil {
		t.Fatal(err)
	}
	if err := RegisterAssistantL1Tools(reg, deps, nil); err != nil {
		t.Fatal(err)
	}
	grouped := map[string]bool{"doc_read": false, "doc_list": false, "doc_grep": false, "sync_platform_kb": false, "search_platform_kb": false}
	for id := range grouped {
		e, ok := reg.Get(id)
		if !ok {
			t.Fatalf("工具 %s 应已注册", id)
		}
		if e.Group != ToolGroupPlatformKnowledge {
			t.Fatalf("%s 应标注 %q 分组: %q", id, ToolGroupPlatformKnowledge, e.Group)
		}
		grouped[id] = true
	}
	// 非知识类工具不标注分组
	for _, id := range []string{"list_agents", "list_kbs", "get_assistant_config"} {
		if e, _ := reg.Get(id); e.Group != "" {
			t.Fatalf("%s 不应有分组: %q", id, e.Group)
		}
	}
}
