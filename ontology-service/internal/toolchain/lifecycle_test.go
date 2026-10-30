package toolchain

// REQ-540/M347 lifecycle plan 扩面单测（httptest 假运行平面，零外部依赖——沿 toolchain_test.go 先例）：
// ①pin 对账——漂移基准按 pin 版本（B 升级后重装载仍取 pin 版本=不误报 reload，显式刷新才漂移）；
// ②「pin vs 头版本」advisory 提示（不产生动作）；③闭包补全本体进漂移对比（loaded 快照键
// 超出声明集）；④pin 与加载快照不一致时 reload 提示按 pin 口径。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func planStub(t *testing.T, profilesJSON string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime-profiles" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(profilesJSON))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func planProfile(id string, ontologies []string, status, loaded string) string {
	b, _ := json.Marshal(map[string]any{
		"id": id, "name": id + "-名", "engine": "oxigraph", "ontology_ids": ontologies,
		"status": status, "loaded_versions": loaded,
	})
	return string(b)
}

// 验收锚点场景：B 升级（头 v5）后，A 的方案重装载仍取 pin 版本 v2——
// plan 对 pin 本体不产生 reload 动作（漂移基准=pin 而非头版本），只出 advisory 提示。
func TestPlanPinBlocksHeadDrift_Req540(t *testing.T) {
	url := planStub(t, "["+planProfile("pA", []string{"A"}, "running", `{"A":3,"B":2}`)+"]")
	// 方案声明 A，装载快照含闭包补全 B（pin v2）；B 头版本已升到 v5
	plan, err := Plan(context.Background(), url, map[string]int{"A": 3, "B": 5}, []PinEdge{{FromOntoID: "A", ToOntoID: "B", PinVersion: 2}})
	if err != nil {
		t.Fatal(err)
	}
	st := plan.Profiles[0]
	if st.Pinned["B"] != 2 {
		t.Fatalf("生效 pin 应为 2: %+v", st.Pinned)
	}
	if len(st.Closure) != 1 || st.Closure[0] != "B" {
		t.Fatalf("闭包补全应含 B: %v", st.Closure)
	}
	if len(st.PinDrifts) != 1 || st.PinDrifts[0] != (PinDrift{OntologyID: "B", PinVersion: 2, HeadVersion: 5}) {
		t.Fatalf("pin vs 头版本提示不符: %+v", st.PinDrifts)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("pin 对账一致（B 加载 v2=pin v2）不应产生动作（显式刷新才漂移）: %+v", plan.Actions)
	}
	if st.Current["B"] != 5 {
		t.Fatalf("Current 应呈现头版本 5: %+v", st.Current)
	}
}

// 无 pin 的闭包补全本体进漂移对比（缺口 3 后半）：B 头版本超前 → reload 动作。
func TestPlanClosureOntologyDrifts_Req540(t *testing.T) {
	url := planStub(t, "["+planProfile("pA", []string{"A"}, "running", `{"A":3,"B":2}`)+"]")
	plan, err := Plan(context.Background(), url, map[string]int{"A": 3, "B": 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st := plan.Profiles[0]
	if len(st.Drifted) != 1 || st.Drifted[0] != "B" {
		t.Fatalf("闭包补全本体 B 应入漂移清单: %v", st.Drifted)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Action != "reload" {
		t.Fatalf("应产生 reload 动作: %+v", plan.Actions)
	}
	if st.Closure[0] != "B" {
		t.Fatalf("Closure 可观测面应含 B: %v", st.Closure)
	}
}

// pin 与加载快照不一致（pin 刷新后未重装载）：漂移基准=pin → reload 动作+pin 口径原因。
func TestPlanPinMismatchReload_Req540(t *testing.T) {
	url := planStub(t, "["+planProfile("pA", []string{"A"}, "running", `{"A":3,"B":2}`)+"]")
	plan, err := Plan(context.Background(), url, map[string]int{"A": 3, "B": 2}, []PinEdge{{FromOntoID: "A", ToOntoID: "B", PinVersion: 4}})
	if err != nil {
		t.Fatal(err)
	}
	st := plan.Profiles[0]
	if st.Pinned["B"] != 4 {
		t.Fatalf("生效 pin 应为 4: %+v", st.Pinned)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Action != "reload" {
		t.Fatalf("pin 与加载快照不一致应 reload: %+v", plan.Actions)
	}
	if !containsStr(plan.Actions[0].Reason, "pin 版本不一致") || !containsStr(plan.Actions[0].Reason, "B") {
		t.Fatalf("reload 原因应按 pin 口径: %q", plan.Actions[0].Reason)
	}
	// pin=4 > 头版本 2 不可能（版本单调）——PinDrifts 应为空
	if len(st.PinDrifts) != 0 {
		t.Fatalf("pin 超前头版本不应出提示: %+v", st.PinDrifts)
	}
}

// 引用方不在装载组内的 pin 边不生效；无快照旧数据 UnknownDrift 语义维持。
func TestPlanPinScopeAndUnknownDrift_Req540(t *testing.T) {
	// pin 边 from 端（C）不在方案内 → 不影响对账：B 按头版本漂移
	url := planStub(t, "["+planProfile("pA", []string{"A"}, "running", `{"A":3,"B":2}`)+"]")
	plan, err := Plan(context.Background(), url, map[string]int{"A": 3, "B": 5}, []PinEdge{{FromOntoID: "C", ToOntoID: "B", PinVersion: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Profiles[0].Pinned) != 0 {
		t.Fatalf("组外 pin 边不生效: %+v", plan.Profiles[0].Pinned)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Action != "reload" {
		t.Fatalf("无 pin 时 B 头版本漂移应 reload: %+v", plan.Actions)
	}
	// 旧数据无快照：UnknownDrift 提示 reload 建立基线（不误报版本漂移）
	url2 := planStub(t, "["+planProfile("pB", []string{"A"}, "running", ``)+"]")
	plan2, err := Plan(context.Background(), url2, map[string]int{"A": 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan2.Profiles[0].UnknownDrift {
		t.Fatal("无快照应 UnknownDrift")
	}
	if len(plan2.Actions) != 1 || !containsStr(plan2.Actions[0].Reason, "基线") {
		t.Fatalf("UnknownDrift 应建议重载建立基线: %+v", plan2.Actions)
	}
}

// 同目标多 pin 边取最低（与运行面 expandClosure 收敛口径一致）。
func TestPlanPinMinAggregation_Req540(t *testing.T) {
	url := planStub(t, "["+planProfile("pA", []string{"A", "C"}, "running", `{"A":1,"C":1,"B":1}`)+"]")
	plan, err := Plan(context.Background(), url, map[string]int{"A": 1, "C": 1, "B": 1}, []PinEdge{
		{FromOntoID: "A", ToOntoID: "B", PinVersion: 3},
		{FromOntoID: "C", ToOntoID: "B", PinVersion: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Profiles[0].Pinned["B"]; got != 2 {
		t.Fatalf("多 pin 边应取最低 2: %d", got)
	}
}

// 评审修正场景：装载快照里的本体已从构建平面删除（CurrentVersions 查不到得 0）——
// 不判「版本落后」漂移（反事实），单列 Missing 准确归因，reload 原因明示装载门禁将阻断。
func TestPlanMissingOntologyAccurateReason_Req540(t *testing.T) {
	url := planStub(t, "["+planProfile("pA", []string{"A"}, "running", `{"A":3,"B":2}`)+"]")
	plan, err := Plan(context.Background(), url, map[string]int{"A": 3}, nil) // B 不在构建平面（已删）
	if err != nil {
		t.Fatal(err)
	}
	st := plan.Profiles[0]
	if len(st.Missing) != 1 || st.Missing[0] != "B" {
		t.Fatalf("已删本体应单列 Missing: %+v", st.Missing)
	}
	for _, d := range st.Drifted {
		if d == "B" {
			t.Fatalf("已删本体不应入版本漂移清单（对比无意义）: %v", st.Drifted)
		}
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Action != "reload" {
		t.Fatalf("应恰一条 reload 动作: %+v", plan.Actions)
	}
	if !containsStr(plan.Actions[0].Reason, "已从构建平面删除") || !containsStr(plan.Actions[0].Reason, "装载门禁") {
		t.Fatalf("原因应准确归因删除+门禁阻断预期: %q", plan.Actions[0].Reason)
	}
}

// 二轮评审修正：Missing 与 Drifted 并存合并为同方案单条 reload 动作（两条独立动作会
// apply 两次=同方案连续两轮 Stop/Start 重复重载）。
func TestPlanMissingAndDriftedSingleAction_Req540(t *testing.T) {
	url := planStub(t, "["+planProfile("pA", []string{"A"}, "running", `{"A":3,"B":2}`)+"]")
	plan, err := Plan(context.Background(), url, map[string]int{"A": 5}, nil) // A 版本漂移 + B 已删
	if err != nil {
		t.Fatal(err)
	}
	st := plan.Profiles[0]
	if len(st.Missing) != 1 || st.Missing[0] != "B" {
		t.Fatalf("Missing 应为 [B]: %+v", st.Missing)
	}
	if len(st.Drifted) != 1 || st.Drifted[0] != "A" {
		t.Fatalf("Drifted 应为 [A]: %v", st.Drifted)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("同方案应合并为单条动作（防 apply 双轮重载）: %+v", plan.Actions)
	}
	if plan.Actions[0].Action != "reload" {
		t.Fatalf("合并动作应为 reload: %+v", plan.Actions[0])
	}
	if !containsStr(plan.Actions[0].Reason, "已从构建平面删除") || !containsStr(plan.Actions[0].Reason, "spec 版本漂移") {
		t.Fatalf("合并原因应同时含 Missing 与 Drifted 两段: %q", plan.Actions[0].Reason)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
