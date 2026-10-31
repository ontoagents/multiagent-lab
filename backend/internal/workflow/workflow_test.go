package workflow

// REQ-562/M369 WF-① 单测（109 号验收句①②③钉桩）：合法 DAG 过/非法逐类人话 400/变量 cap 截断
// 注记/预览 JSON 契约形态。
import (
	"encoding/json"
	"strings"
	"testing"
)

func validSpec() *Spec {
	return &Spec{
		Name: "冒烟计划",
		Nodes: []Node{
			{ID: "n1", Type: NodeLLM, Name: "调研", Instruction: "调研主题", VarWrites: []string{"topic"}},
			{ID: "n2", Type: NodeLLM, Name: "写作", Instruction: "按调研写稿"},
			{ID: "n3", Type: NodeGate, Name: "门禁"},
		},
		Edges: []Edge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
		},
		Vars: []VarSpec{{Name: "topic", Type: "string"}},
	}
}

func TestValidate_Req562(t *testing.T) {
	// ① 合法 DAG（顺序+条件边+并行组）通过
	s := validSpec()
	if err := s.Validate(); err != nil {
		t.Fatalf("合法计划应通过: %v", err)
	}
	// 并行组（两起头节点）+条件边
	p := &Spec{
		Nodes: []Node{
			{ID: "a", Type: NodeLLM, Name: "A"},
			{ID: "b", Type: NodeTool, Tool: "http_fetch", Name: "B"},
			{ID: "c", Type: NodeLLM, Name: "C", Join: JoinAny},
		},
		Edges: []Edge{
			{From: "a", To: "c", Condition: "var:flag==ok"},
			{From: "b", To: "c"},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("并行+条件边计划应通过: %v", err)
	}
	// ② 非法逐类人话
	if err := (&Spec{}).Validate(); err == nil || !strings.Contains(err.Error(), "无节点") {
		t.Fatalf("空计划人话: %v", err)
	}
	dup := validSpec()
	dup.Nodes = append(dup.Nodes, Node{ID: "n1", Type: NodeLLM})
	if err := dup.Validate(); err == nil || !strings.Contains(err.Error(), "id 重复") {
		t.Fatalf("重复 id 人话: %v", err)
	}
	badType := validSpec()
	badType.Nodes[0].Type = "magic"
	if err := badType.Validate(); err == nil || !strings.Contains(err.Error(), "类型") {
		t.Fatalf("非法类型人话: %v", err)
	}
	dangling := validSpec()
	dangling.Edges = append(dangling.Edges, Edge{From: "ghost", To: "n2"})
	if err := dangling.Validate(); err == nil || !strings.Contains(err.Error(), "悬空边") {
		t.Fatalf("悬空边人话: %v", err)
	}
	joinMiss := validSpec()
	joinMiss.Edges = append(joinMiss.Edges, Edge{From: "n3", To: "n2"}) // n2 入度=2 无 join
	if err := joinMiss.Validate(); err == nil || !strings.Contains(err.Error(), "join 语义") {
		t.Fatalf("join 缺条件人话: %v", err)
	}
	badVar := validSpec()
	badVar.Nodes[1].VarWrites = []string{"ghost_var"}
	if err := badVar.Validate(); err == nil || !strings.Contains(err.Error(), "未声明变量") {
		t.Fatalf("悬空写人话: %v", err)
	}
	badVarType := validSpec()
	badVarType.Vars[0].Type = "int64"
	if err := badVarType.Validate(); err == nil || !strings.Contains(err.Error(), "类型 %q 非法") && !strings.Contains(err.Error(), "非法") {
		t.Fatalf("变量类型非法人话: %v", err)
	}
	// 环
	cycle := validSpec()
	cycle.Edges = append(cycle.Edges, Edge{From: "n3", To: "n1"})
	if err := cycle.Validate(); err == nil || !strings.Contains(err.Error(), "含环") {
		t.Fatalf("环人话: %v", err)
	}
	// 拓扑序
	if ord := validSpec().TopoOrder(); ord == nil || len(ord) != 3 || ord[0] != "n1" {
		t.Fatalf("拓扑序失真: %v", ord)
	}
}

func TestVarTable_Req562(t *testing.T) {
	s := validSpec()
	tbl := NewVarTable()
	// 正常写
	note, err := tbl.Set(s, "topic", "n1", "吃了吗")
	if err != nil || note != "" {
		t.Fatalf("正常写失败: %v %q", err, note)
	}
	if v, ok := tbl.Get("topic"); !ok || !strings.Contains(string(v), "吃了吗") {
		t.Fatalf("读回失真: %s", v)
	}
	// 未声明变量防御性拒绝
	if _, err := tbl.Set(s, "ghost", "n1", "x"); err == nil {
		t.Fatal("未声明变量应拒")
	}
	// 超单变量 cap → 截断+注记（验收②）
	big := strings.Repeat("长", VarMaxSize) // 每 rune 3B —— 必超 8KB
	note, err = tbl.Set(s, "topic", "n1", big)
	if err != nil {
		t.Fatalf("超限写应截断不报错: %v", err)
	}
	if note == "" || !strings.Contains(note, "已截断") {
		t.Fatalf("应有截断注记: %q", note)
	}
	if len(tbl.Notes) == 0 {
		t.Fatal("注记应累积（Runner 转 run.warning 数据源）")
	}
	if v, _ := tbl.Get("topic"); len(v) > VarMaxSize {
		t.Fatalf("截断未生效: %d", len(v))
	}
	// MaxSize 覆写
	s2 := &Spec{Vars: []VarSpec{{Name: "big", Type: "string", MaxSize: 2 * 1024}}}
	t2 := NewVarTable()
	if _, err := t2.Set(s2, "big", "n1", strings.Repeat("x", 4096)); err != nil {
		t.Fatal(err)
	}
	if v, _ := t2.Get("big"); len(v) > 2*1024 {
		t.Fatalf("覆写 cap 未生效: %d", len(v))
	}
}

func TestPreview_Req562(t *testing.T) {
	s := &Spec{
		Name: "并行预览",
		Nodes: []Node{
			{ID: "start", Type: NodeLLM, Name: "起点"},
			{ID: "p1", Type: NodeLLM, Name: "并行甲"},
			{ID: "p2", Type: NodeTool, Tool: "http_fetch", Name: "并行乙"},
			{ID: "join", Type: NodeLLM, Name: "汇合", Join: JoinAll},
		},
		Edges: []Edge{
			{From: "start", To: "p1"},
			{From: "start", To: "p2"},
			{From: "p1", To: "join"},
			{From: "p2", To: "join"},
		},
	}
	b, err := PreviewJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	var p Preview
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Nodes) != 4 || p.Depth != 2 {
		t.Fatalf("预览失真: %d 节点 depth=%d", len(p.Nodes), p.Depth)
	}
	// 并行组标注：p1/p2 同深度
	par := map[string]bool{}
	for _, n := range p.Nodes {
		if n.Depth == 1 {
			par[n.ID] = n.Parallel
		}
	}
	if !par["p1"] || !par["p2"] {
		t.Fatalf("同深度应标 parallel: %v", par)
	}
	// 条件边透传（WF-⑦「如果…就…」问句素材）
	cp := &Spec{
		Nodes: []Node{{ID: "a", Type: NodeLLM}, {ID: "b", Type: NodeLLM}},
		Edges: []Edge{{From: "a", To: "b", Condition: "var:flag==ok"}},
	}
	b2, _ := PreviewJSON(cp)
	if !strings.Contains(string(b2), "var:flag==ok") {
		t.Fatalf("条件边应入预览: %s", b2)
	}
	// 非法计划预览 400 形态
	if _, err := PreviewJSON(&Spec{}); err == nil {
		t.Fatal("非法计划预览应报错（调用方 400）")
	}
}
