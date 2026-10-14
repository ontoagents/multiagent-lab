package companion

// REQ-313 概念画像检索单元零依赖单测（沿 req194_test 纯函数先例）：
//   ①buildConceptProfiles：属类后代闭包（BFS 防环）/别名/一阶关系方向/无增广同形/限量治理；
//   ②retrievalCorpus：画像命中用画像、不可用精确回退旧格式；
//   ③parseRelRows/parseAliasPairs 投影解析。

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildConceptProfiles(t *testing.T) {
	defs := map[string]string{
		"容器编排":  "容器化应用的编排与调度",
		"工作负载":  "集群中运行的应用形态",
		"Pod":    "最小调度单元",
		"HPA":    "水平自动扩缩容",
		"K8s":    "容器编排平台",
	}
	rels := []profileRelRow{
		{Src: "Pod", Rel: "属于", Dst: "工作负载"},
		{Src: "HPA", Rel: "属于", Dst: "工作负载"},
		{Src: "工作负载", Rel: "属于", Dst: "容器编排"},
		{Src: "容器编排", Rel: "部署于", Dst: "K8s"},
		{Src: "Pod", Rel: "属于", Dst: "工作负载"}, // 重复边去重
	}
	aliases := []profileAliasRow{{Owner: "容器编排", Alias: "K8s 编排"}}
	p := buildConceptProfiles(defs, rels, aliases)

	// 后代闭包：容器编排 → 工作负载 → {Pod,HPA}（两跳闭包）
	cp := p["容器编排"]
	for _, want := range []string{"又称 K8s 编排", "「部署于」→ K8s", "涵盖：", "「工作负载」", "「Pod」", "「HPA」", "水平自动扩缩容"} {
		if !strings.Contains(cp, want) {
			t.Fatalf("容器编排画像缺 %q：%s", want, cp)
		}
	}
	// 后代边不入关系摘要（工作负载→容器编排 的入向属类边由涵盖承载）
	wl := p["工作负载"]
	if strings.Contains(wl, "→「属于」") {
		t.Fatalf("入向属类边不应进关系摘要：%s", wl)
	}
	for _, want := range []string{"「属于」→ 容器编排", "「Pod」", "「HPA」"} {
		if !strings.Contains(wl, want) {
			t.Fatalf("工作负载画像缺 %q：%s", want, wl)
		}
	}
	// 出向属类边保留（父语义有价值）
	if !strings.Contains(p["Pod"], "「属于」→ 工作负载") {
		t.Fatalf("Pod 画像应含出向属类边：%s", p["Pod"])
	}
}

func TestBuildConceptProfilesNoAugmentSameShape(t *testing.T) {
	defs := map[string]string{"A": "定义 A", "B": ""}
	p := buildConceptProfiles(defs, nil, nil)
	if p["A"] != "A\n定义 A" {
		t.Fatalf("无增广应与旧语料同形：%q", p["A"])
	}
	if p["B"] != "B" {
		t.Fatalf("无定义实语料应为裸标签：%q", p["B"])
	}
}

func TestBuildConceptProfilesCycleSafe(t *testing.T) {
	defs := map[string]string{"X": "x", "Y": "y"}
	rels := []profileRelRow{{Src: "X", Rel: "属于", Dst: "Y"}, {Src: "Y", Rel: "属于", Dst: "X"}}
	p := buildConceptProfiles(defs, rels, nil)
	if p["X"] == "" || p["Y"] == "" {
		t.Fatal("环状属类不应中断画像构建")
	}
	if strings.Count(p["X"], "「Y」") > 2 {
		t.Fatalf("闭包应防环限量：%s", p["X"])
	}
}

func TestBuildConceptProfilesCaps(t *testing.T) {
	defs := map[string]string{"根": "根类"}
	rels := make([]profileRelRow, 0, 20)
	for i := 0; i < 12; i++ {
		defs[childName(i)] = "子类定义内容"
		rels = append(rels, profileRelRow{Src: childName(i), Rel: "属于", Dst: "根"})
	}
	p := buildConceptProfiles(defs, rels, nil)
	if got := strings.Count(p["根"], "「"); got != profileMaxDescendants {
		t.Fatalf("后代应限量 %d，实际 %d", profileMaxDescendants, got)
	}
	// 总长上限
	long := map[string]string{"长": strings.Repeat("长", 3000)}
	p2 := buildConceptProfiles(long, nil, nil)
	if len([]rune(p2["长"])) > profileMaxRunes {
		t.Fatalf("画像应截断至 %d runes，实际 %d", profileMaxRunes, len([]rune(p2["长"])))
	}
}

func childName(i int) string { return "子" + strings.Repeat("x", i%3) + string(rune('A'+i)) }

func TestRetrievalCorpusFallback(t *testing.T) {
	labels := []string{"A", "alias-a"}
	defs := map[string]string{"A": "定义 A"}
	// 画像不可用（nil）→ 精确回退旧格式
	c := retrievalCorpus(labels, defs, nil)
	if c["A"] != "A\n定义 A" || c["alias-a"] != "alias-a" {
		t.Fatalf("回退语料不符：%v", c)
	}
	// 画像命中 → 用画像全文
	c2 := retrievalCorpus(labels, defs, map[string]string{"A": "A\n定义 A\n涵盖：「B」"})
	if c2["A"] != "A\n定义 A\n涵盖：「B」" || c2["alias-a"] != "alias-a" {
		t.Fatalf("画像语料接线不符：%v", c2)
	}
}

func TestParseProfileRows(t *testing.T) {
	sparqlJSON := func(bindings string) []byte {
		return []byte(`{"results":{"bindings":[` + bindings + `]}}`)
	}
	rows := parseRelRows(sparqlJSON(`{"src":{"value":"Pod"},"rel":{"value":"属于"},"dst":{"value":"工作负载"}}`))
	if len(rows) != 1 || rows[0].Src != "Pod" || rows[0].Rel != "属于" || rows[0].Dst != "工作负载" {
		t.Fatalf("关系行解析不符：%v", rows)
	}
	als := parseAliasPairs(sparqlJSON(`{"label":{"value":"容器编排"},"alias":{"value":"K8s 编排"}}`))
	if len(als) != 1 || als[0].Owner != "容器编排" || als[0].Alias != "K8s 编排" {
		t.Fatalf("别名行解析不符：%v", als)
	}
	if parseRelRows([]byte("not json")) != nil {
		t.Fatal("坏 JSON 应返回 nil")
	}
	_ = json.Marshal // 保持 import（结构体标签编译校验）
}
