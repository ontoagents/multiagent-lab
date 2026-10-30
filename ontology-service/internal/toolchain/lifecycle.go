package toolchain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// PinEdge 引用 pin 边（REQ-540③ pin 对账数据源；与 repo.PinEdge 同构投影——
// toolchain 包不依赖 repo，REST 层转换注入）。
type PinEdge struct {
	FromOntoID string
	ToOntoID   string
	PinVersion int
}

// ---------------------------------------------------------------------------
// REQ-155/M-O15 阶段二：本体方案生命周期（Terraform 式 plan/apply，monitor=plan 的只读形态）。
//   期望态 = 方案配置本身（runtime_profile 声明的引擎与本体集合，全部声明为期望运行）；
//   实际态 = 运行平面回报的 status + 启动/重载时记录的加载版本快照（loaded_versions）；
//   plan   = 期望-实际差异 → 动作清单（start=未运行 / reload=运行中但任一本体 spec 版本超前于加载快照=漂移）；
//   apply  = 逐项执行动作（经主平台反代调运行平面 start/reload），stop 不入 plan（声明式语义：摘除方案=删配置）。
// 漂移信号依赖 loaded_versions（M-O15 阶段二随本交付引入；未记录快照的旧运行方案标注 unknown，不误报）。
// ---------------------------------------------------------------------------

// LifecycleAction 单条计划动作。
type LifecycleAction struct {
	ProfileID   string `json:"profile_id"`
	ProfileName string `json:"profile_name"`
	Engine      string `json:"engine,omitempty"`
	Action      string `json:"action"` // start | reload
	Reason      string `json:"reason"` // 人类可读原因（ drifted: spec vX → vY 等）
}

// PinDrift REQ-540③：「pin 版本 vs 头版本」漂移提示（advisory——不产生动作；处置=
// 资产页刷新 pin（PUT external-refs/{refId}/pin）或保持 pin，刷新后重装载才取新版本）。
type PinDrift struct {
	OntologyID  string `json:"ontology_id"`
	PinVersion  int    `json:"pin_version"`
	HeadVersion int    `json:"head_version"`
}

// LifecyclePlan 生命周期计划。
type LifecyclePlan struct {
	GeneratedAt string            `json:"generated_at"`
	Profiles    []LifecycleStatus `json:"profiles"`
	Actions     []LifecycleAction `json:"actions"`
}

// LifecycleStatus 单方案状态行（面板渲染）。
type LifecycleStatus struct {
	ProfileID    string         `json:"profile_id"`
	ProfileName  string         `json:"profile_name"`
	Engine       string         `json:"engine"`
	Status       string         `json:"status"` // running | stopped | error | starting | created
	Ontologies   []string       `json:"ontologies"`
	Loaded       map[string]int `json:"loaded"`            // 加载快照（可能缺）
	Current      map[string]int `json:"current"`           // 当前 spec 版本
	Drifted      []string       `json:"drifted,omitempty"` // 漂移本体名（版本超前）
	UnknownDrift bool           `json:"unknown_drift"`     // 运行中但无快照（旧数据，建议 reload 建立基线）
	LastError    string         `json:"last_error,omitempty"`
	// REQ-540③④：pin 对账与闭包补全可见性。
	Pinned    map[string]int `json:"pinned,omitempty"`     // 本方案生效 pin（目标本体 → pin 版本；多登记取最低）
	Closure   []string       `json:"closure,omitempty"`    // 闭包补全本体（装载快照含而声明集不含——缺口 3 可观测面）
	PinDrifts []PinDrift     `json:"pin_drifts,omitempty"` // pin 落后头版本提示（advisory，不产生动作）
}

type profileDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Engine         string   `json:"engine"`
	OntologyIDs    []string `json:"ontology_ids"`
	Status         string   `json:"status"`
	LastError      string   `json:"last_error,omitempty"`
	LoadedVersions string   `json:"loaded_versions,omitempty"`
}

// Plan 拉取运行平面方案清单并与当前 spec 版本对比，产出计划（HTTP 只读）。
// REQ-540/M347 扩面：①pinEdges 注入 pin 对账——漂移基准按 pin（登记钉住的版本）而非
// 头版本，pin 落后头版本出 advisory 提示（可刷新或保持，不产生动作）；②对账范围扩为
// 声明集 ∪ loaded_versions 键（闭包补全本体进漂移对比，修运行面缺口 3 后半）。
func Plan(ctx context.Context, platformURL string, currentVersions map[string]int, pinEdges []PinEdge) (*LifecyclePlan, error) {
	profiles, err := fetchProfiles(ctx, platformURL)
	if err != nil {
		return nil, err
	}
	plan := &LifecyclePlan{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Profiles: []LifecycleStatus{}, Actions: []LifecycleAction{}}
	for _, p := range profiles {
		st := LifecycleStatus{
			ProfileID: p.ID, ProfileName: p.Name, Engine: p.Engine, Status: p.Status,
			Ontologies: p.OntologyIDs, Current: map[string]int{}, Loaded: map[string]int{},
			Pinned:    map[string]int{},
			LastError: p.LastError,
		}
		if p.LoadedVersions != "" {
			_ = json.Unmarshal([]byte(p.LoadedVersions), &st.Loaded)
		}
		// 对账范围=声明集 ∪ 装载快照键（REQ-540④：闭包补全本体进漂移对比）
		declaredSet := map[string]bool{}
		for _, oid := range p.OntologyIDs {
			declaredSet[oid] = true
		}
		reconcile := append([]string(nil), p.OntologyIDs...)
		loadedOnly := map[string]bool{}
		for oid := range st.Loaded {
			if !declaredSet[oid] {
				loadedOnly[oid] = true
				reconcile = append(reconcile, oid)
			}
		}
		sort.Strings(reconcile[len(p.OntologyIDs):]) // 闭包补全部分确定性序
		for oid := range loadedOnly {
			st.Closure = append(st.Closure, oid)
		}
		sort.Strings(st.Closure)
		for _, oid := range reconcile {
			st.Current[oid] = currentVersions[oid]
		}
		// 本方案生效 pin（REQ-540③）：pin 边 from 端在本方案对账集内才生效（引用方在装载
		// 组里，其 pin 才约束本次装载）；同目标多边取最低=最保守（与运行面 expandClosure 一致）。
		inSet := map[string]bool{}
		for _, oid := range reconcile {
			inSet[oid] = true
		}
		for _, e := range pinEdges {
			if e.PinVersion <= 0 || !inSet[e.FromOntoID] || !inSet[e.ToOntoID] {
				continue
			}
			if prev, ok := st.Pinned[e.ToOntoID]; !ok || e.PinVersion < prev {
				st.Pinned[e.ToOntoID] = e.PinVersion
			}
		}
		switch p.Status {
		case "running":
			var pinMismatch []string
			for _, oid := range reconcile {
				loaded, ok := st.Loaded[oid]
				if !ok {
					st.UnknownDrift = true // 无基线快照（旧运行），不误报版本漂移
					continue
				}
				expect := st.Current[oid] // 浮动头版本（现状语义）
				if pin := st.Pinned[oid]; pin > 0 {
					expect = pin // pin 对账基准=登记钉住的版本（REQ-540③）
				}
				if loaded != expect {
					st.Drifted = append(st.Drifted, oid)
					if pin := st.Pinned[oid]; pin > 0 {
						pinMismatch = append(pinMismatch, oid)
					}
				}
				// pin 落后头版本 → advisory 提示（不产生动作：重装载仍取 pin，刷新 pin 才漂移）
				if pin := st.Pinned[oid]; pin > 0 && st.Current[oid] > pin {
					st.PinDrifts = append(st.PinDrifts, PinDrift{OntologyID: oid, PinVersion: pin, HeadVersion: st.Current[oid]})
				}
			}
			if len(st.Drifted) > 0 {
				reason := fmt.Sprintf("spec 版本漂移：%s（加载快照落后于当前版本）", strings.Join(st.Drifted, "、"))
				if len(pinMismatch) > 0 {
					reason += fmt.Sprintf("；%s 加载快照与登记 pin 版本不一致（重装载按 pin 取版本）", strings.Join(pinMismatch, "、"))
				}
				plan.Actions = append(plan.Actions, LifecycleAction{
					ProfileID: p.ID, ProfileName: p.Name, Engine: p.Engine, Action: "reload",
					Reason: reason,
				})
			} else if st.UnknownDrift {
				plan.Actions = append(plan.Actions, LifecycleAction{
					ProfileID: p.ID, ProfileName: p.Name, Engine: p.Engine, Action: "reload",
					Reason: "运行中但无加载版本基线（旧数据）——重载建立快照",
				})
			}
		case "stopped", "created", "error":
			if len(p.OntologyIDs) > 0 {
				plan.Actions = append(plan.Actions, LifecycleAction{
					ProfileID: p.ID, ProfileName: p.Name, Engine: p.Engine, Action: "start",
					Reason: map[string]string{
						"stopped": "已声明未运行", "created": "已创建未启动",
						"error": "上次启动失败（" + p.LastError + "）",
					}[p.Status],
				})
			}
		}
		plan.Profiles = append(plan.Profiles, st)
	}
	return plan, nil
}

// ApplyAction 执行一条动作（start/reload 经主平台反代调运行平面）。
func ApplyAction(ctx context.Context, platformURL string, a LifecycleAction) error {
	verb := a.Action
	if verb != "start" && verb != "reload" {
		return fmt.Errorf("不支持的动作 %q（仅 start|reload）", verb)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(platformURL, "/")+fmt.Sprintf("/api/runtime-profiles/%s/%s", a.ProfileID, verb),
		strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("运行平面不可达: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s 失败: %s", verb, a.ProfileID, strings.TrimSpace(string(b)))
	}
	return nil
}

func fetchProfiles(ctx context.Context, platformURL string) ([]profileDTO, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(platformURL, "/")+"/api/runtime-profiles", nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("运行平面不可达（经主平台）: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("运行平面 %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out []profileDTO
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("方案清单解析失败: %w", err)
	}
	return out, nil
}
