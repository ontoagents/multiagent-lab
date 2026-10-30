package repo

// REQ-542/M349：构建面只读闭包预览（103 号《本体依赖管理与仓库服务调研》§6.5 步骤二前半/
// §7.5-2③，修痛点 P2 剩余面）——运行面 expandClosure（runtime-manager closure.go：BFS 沿仓内
// external_refs import/reuse + owl:imports 哨兵 urn:o:{id}:，环 visited 收敛=并集装载语义）
// 算法前移为构建面只读预览，算法同构（环收敛/哨兵解析/pin 收敛口径一致）。
// D-O27 边界扩写非反转（03 §5 行注/§2.72 口径注）：只读端点不回写不阻断、不承载装载语义——
// 悬空引用如实报告不拦截（装载阻断仍是运行面门禁职责），构建面 merge/校验/qualitygate
// 「不解析引用」既有语义与远端 IRI 拉取·联邦查询不做两项均不触碰。
// 三出料=①全量依赖闭包图（nodes+edges 可视化数据）②解析后 schema 预览（引用方+被引方按
// REQ-540 pin/head 解析合成的完整 schema 视图；ABox 实例不入=预览面是 schema 非数据）
// ③循环引用专项报告（彩色 DFS 回边环，环语义=并集装载不受影响，报告仅为可观测）。
// 版本解析按 REQ-540 pin 语义：同目标多登记取最低 pin（最保守、不限 ref_kind——登记即 pin
// 载体），无 pin=头版本；同目标多版本 pin 冲突集如实呈现（与运行面 expandClosure PinConflicts
// 同构）。与 REQ-278 impact（版本 diff+引用波及面）为扩面衔接非撞号。

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

// ClosureNode 闭包图节点（根本体+闭包内每本体，BFS 发现序、root 在首）。
type ClosureNode struct {
	OntoID     string `json:"onto_id"`
	Name       string `json:"name,omitempty"`
	Resolution string `json:"resolution"`            // root | head | pin | missing
	Version    int    `json:"version"`               // 解析时点生效版本（missing=0）
	Concepts   int    `json:"concepts"`              // 解析时点 spec 概念数（missing=0）
	SpecSHA256 string `json:"spec_sha256,omitempty"` // 解析时点内容指纹（缺失为空不虚构）
}

// ClosureEdge 闭包图边（装载边：import/reuse 仓内引用 + owl:imports 哨兵；alignment 纯登记
// 不产装载边不入闭包）。Dangling 为报告字段——预览不阻断（装载阻断是运行面门禁职责）。
type ClosureEdge struct {
	FromOntoID  string `json:"from_onto_id"`
	ToOntoID    string `json:"to_onto_id"`
	RefKind     string `json:"ref_kind"` // import | reuse（owl:imports 哨兵=import）
	FromConcept string `json:"from_concept,omitempty"`
	ToConcept   string `json:"to_concept,omitempty"`
	ViaImports  bool   `json:"via_imports,omitempty"` // 经 owl:imports 哨兵登记（REQ-262）
	Dangling    string `json:"dangling,omitempty"`    // ""|missing_ontology|missing_concept
}

// ClosureDangling 悬空引用专项条目（预览报告，不阻断）。
type ClosureDangling struct {
	OntoID string `json:"onto_id"` // 引用方
	RefID  string `json:"ref_id"`
	Code   string `json:"code"` // missing_ontology | missing_concept
	Detail string `json:"detail"`
}

// SchemaSection 解析后 schema 预览分节（按本体一节，标注解析来源；同名概念跨本体冲突由
// NameConflicts 呈现——分节保来源边界，平铺名单给合成视图）。
type SchemaSection struct {
	OntoID         string                 `json:"onto_id"`
	Resolution     string                 `json:"resolution"` // root | head | pin
	Version        int                    `json:"version"`
	Concepts       []pkgspec.Concept      `json:"concepts"`
	Relations      []pkgspec.Relation     `json:"relations"`
	DataProperties []pkgspec.DataProperty `json:"data_properties,omitempty"`
}

// SchemaNameConflict 跨本体同名概念（合成视图需人工裁决处；本体按分节序）。
type SchemaNameConflict struct {
	Name    string   `json:"name"`
	OntoIDs []string `json:"onto_ids"`
}

// ResolvedSchema 解析后 schema 预览（引用方+被引方概念合成的完整 schema 视图）。
type ResolvedSchema struct {
	Sections      []SchemaSection      `json:"sections"`
	ConceptNames  []string             `json:"concept_names"` // 合成视图平铺名单（去重升序）
	NameConflicts []SchemaNameConflict `json:"name_conflicts,omitempty"`
	TotalConcepts int                  `json:"total_concepts"`
}

// ClosurePreview 构建面只读闭包预览（三出料集合；BuildClosurePreview 全链无写路径）。
type ClosurePreview struct {
	RootID        string            `json:"root_id"`
	GeneratedAt   string            `json:"generated_at"`
	Nodes         []ClosureNode     `json:"nodes"`
	Edges         []ClosureEdge     `json:"edges"`
	RemoteSkipped []string          `json:"remote_skipped,omitempty"` // 外部 IRI 哨兵（不拉取，D-O27）
	Dangling      []ClosureDangling `json:"dangling,omitempty"`       // 悬空报告（不阻断）
	Pins          map[string]int    `json:"pins,omitempty"`           // REQ-540 pin 解析结果（目标→有效 pin）
	PinConflicts  map[string]string `json:"pin_conflicts,omitempty"`  // 同目标多 pin 冲突集（"v1/v2"）
	Schema        *ResolvedSchema   `json:"schema"`
	Cycles        [][]string        `json:"cycles"` // 循环引用专项报告（回边环路径）
}

// BuildClosurePreview 构建面只读闭包预览：BFS 从根本体沿 import/reuse 仓内引用 + owl:imports
// 哨兵递归补全（visited 收敛=并集语义，与运行面 expandClosure/release manifest 同构），三出料
// 合成返回。悬空引用（目标已删/概念缺失）如实报告不阻断、missing 节点无内容不外扩解析。
// 全链只读：零 INSERT/UPDATE/DELETE（预览零写副作用验收锚点——版本/引用/spec 均不动）。
func (s *Store) BuildClosurePreview(ontoID string) (*ClosurePreview, error) {
	root, err := s.GetOntology(ontoID)
	if err != nil {
		return nil, err
	}
	prev := &ClosurePreview{
		RootID:      ontoID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Nodes:       []ClosureNode{},
		Edges:       []ClosureEdge{},
		Cycles:      [][]string{},
		Schema:      &ResolvedSchema{Sections: []SchemaSection{}, ConceptNames: []string{}},
	}
	visited := map[string]bool{}
	queue := []string{ontoID}
	edges := map[string][]string{} // 环检测用邻接表（仅装载边）
	pinSeen := map[string]map[int]bool{}
	remoteSeen := map[string]bool{}
	var order []string // BFS 发现序（含 root 与 missing 目标——图中如实呈现）
	for len(queue) > 0 {
		oid := queue[0]
		queue = queue[1:]
		if visited[oid] {
			continue
		}
		visited[oid] = true
		order = append(order, oid)
		refs, rerr := s.ListExternalRefs(oid)
		if rerr != nil {
			return nil, fmt.Errorf("闭包预览失败（本体 %s 引用清单读取）: %w", oid, rerr)
		}
		s.AnnotateDangling(refs)
		// 预览确定性排序：ListExternalRefs 按 created_at,ref_id——同秒登记 created_at 相同、
		// ref_id 随机，BFS 发现序会不稳定；预览按（to_onto_id,to_concept,ref_id）重排
		//（仅本函数局部序，不动 ListExternalRefs 既有语义与导出桥消费路径）。
		sort.SliceStable(refs, func(i, j int) bool {
			if refs[i].ToOntoID != refs[j].ToOntoID {
				return refs[i].ToOntoID < refs[j].ToOntoID
			}
			if refs[i].ToConcept != refs[j].ToConcept {
				return refs[i].ToConcept < refs[j].ToConcept
			}
			return refs[i].RefID < refs[j].RefID
		})
		for _, ref := range refs {
			// REQ-540 pin 收敛：同目标多登记取最低（不限 ref_kind，登记即 pin 载体）；
			// 多版本 pin 记冲突集（与运行面 expandClosure 同口径）。
			if ref.ToOntoID != "" && ref.PinVersion > 0 {
				if pinSeen[ref.ToOntoID] == nil {
					pinSeen[ref.ToOntoID] = map[int]bool{}
				}
				pinSeen[ref.ToOntoID][ref.PinVersion] = true
				if p, ok := prev.Pins[ref.ToOntoID]; !ok || ref.PinVersion < p {
					if prev.Pins == nil {
						prev.Pins = map[string]int{}
					}
					prev.Pins[ref.ToOntoID] = ref.PinVersion
				}
			}
			switch {
			case ref.ToOntoID != "" && (ref.RefKind == "import" || ref.RefKind == "reuse"):
				prev.Edges = append(prev.Edges, ClosureEdge{FromOntoID: oid, ToOntoID: ref.ToOntoID,
					RefKind: ref.RefKind, FromConcept: ref.FromConcept, ToConcept: ref.ToConcept,
					Dangling: ref.Dangling})
				if ref.Dangling != "" {
					prev.Dangling = append(prev.Dangling, ClosureDangling{
						OntoID: oid, RefID: ref.RefID, Code: ref.Dangling,
						Detail: fmt.Sprintf("%s:%s →%s:%s 目标%s（预览不阻断；装载由运行面门禁拦截）",
							ref.FromConcept, ref.RefKind, ref.ToOntoID, ref.ToConcept, danglingCodeText(ref.Dangling)),
					})
				}
				edges[oid] = append(edges[oid], ref.ToOntoID)
				if !visited[ref.ToOntoID] {
					queue = append(queue, ref.ToOntoID)
				}
			case ref.ToOntoID == "" && ref.RefKind == "import" && ref.FromConcept == OntologyLevelRef:
				// owl:imports 装载侧哨兵登记（REQ-262）：to_concept=urn:o:{id}: 形态解析仓内目标
				if tgt := urnOntoIDOf(ref.ToConcept); tgt != "" {
					prev.Edges = append(prev.Edges, ClosureEdge{FromOntoID: oid, ToOntoID: tgt,
						RefKind: "import", FromConcept: ref.FromConcept, ToConcept: ref.ToConcept, ViaImports: true})
					edges[oid] = append(edges[oid], tgt)
					if !visited[tgt] {
						queue = append(queue, tgt)
					}
				} else if !remoteSeen[ref.ToConcept] {
					remoteSeen[ref.ToConcept] = true
					prev.RemoteSkipped = append(prev.RemoteSkipped, ref.ToConcept)
				}
			}
		}
	}
	// pin 冲突集（版本升序 "v1/v2"，与运行面同构）
	for t, ps := range pinSeen {
		if len(ps) < 2 {
			continue
		}
		vers := make([]int, 0, len(ps))
		for v := range ps {
			vers = append(vers, v)
		}
		sort.Ints(vers)
		parts := make([]string, 0, len(vers))
		for _, v := range vers {
			parts = append(parts, fmt.Sprintf("v%d", v))
		}
		if prev.PinConflicts == nil {
			prev.PinConflicts = map[string]string{}
		}
		prev.PinConflicts[t] = strings.Join(parts, "/")
	}
	// 节点解析（BFS 发现序）：root=头版本；dep 按 pin/head 取内容——指纹语义与 release manifest
	// 一致（pin=该版本快照 / head=头版本工件；missing 无内容不虚构）。
	for _, oid := range order {
		n := ClosureNode{OntoID: oid}
		if oid == ontoID {
			n.Resolution, n.Version, n.Name = "root", root.Version, root.Name
		} else {
			o, gerr := s.GetOntology(oid)
			if errors.Is(gerr, ErrNotFound) {
				n.Resolution = "missing" // 发布时点/预览时点已删（悬空如实入图，不沿其外扩）
				prev.Nodes = append(prev.Nodes, n)
				continue
			}
			if gerr != nil {
				return nil, gerr
			}
			if p := prev.Pins[oid]; p > 0 {
				n.Resolution, n.Version, n.Name = "pin", p, o.Name
			} else {
				n.Resolution, n.Version, n.Name = "head", o.Version, o.Name
			}
		}
		raw, rerr := s.resolveSpecAt(oid, n.Resolution, n.Version)
		if rerr != nil {
			if !errors.Is(rerr, ErrNotFound) {
				return nil, rerr
			}
			// 工件/快照缺失（仅 original 未归一化等态）：如实缺指纹与 schema 分节，不虚构
		} else {
			n.SpecSHA256 = sha256Hex(raw)
			sec, serr := schemaSection(raw, oid, n.Resolution, n.Version)
			if serr != nil {
				return nil, serr
			}
			n.Concepts = len(sec.Concepts)
			prev.Schema.Sections = append(prev.Schema.Sections, *sec)
		}
		prev.Nodes = append(prev.Nodes, n)
	}
	// 合成视图名单+跨本体同名冲突（分节序遍历保确定性；名单去重升序）
	nameOwner := map[string][]string{}
	seen := map[string]bool{}
	var names []string
	for _, sec := range prev.Schema.Sections {
		for _, c := range sec.Concepts {
			if !seen[c.Name] {
				seen[c.Name] = true
				names = append(names, c.Name)
			}
			nameOwner[c.Name] = append(nameOwner[c.Name], sec.OntoID)
		}
	}
	sort.Strings(names)
	prev.Schema.ConceptNames = names
	for _, n := range names {
		if owners := nameOwner[n]; len(owners) > 1 {
			prev.Schema.NameConflicts = append(prev.Schema.NameConflicts, SchemaNameConflict{Name: n, OntoIDs: owners})
		}
	}
	for _, sec := range prev.Schema.Sections {
		prev.Schema.TotalConcepts += len(sec.Concepts)
	}
	prev.Cycles = findClosureCycles(edges, ontoID)
	return prev, nil
}

// resolveSpecAt 取本体解析时点 spec_json：pin=版本快照（REQ-540 pin 解析点）；
// root/head=头版本 artifact（artifact 恒头版本内容，与 ontology.version 同源）。
func (s *Store) resolveSpecAt(ontoID, resolution string, version int) (string, error) {
	if resolution == "pin" {
		return s.GetVersionSpec(ontoID, version)
	}
	raw, _, err := s.GetArtifact(ontoID, "spec_json")
	return raw, err
}

// schemaSection 解析 spec_json 为 schema 分节（TBox 面：概念/关系/数据属性；ABox 实例与
// CQ 元数据不入=预览面是 schema 非数据；解析失败如实报错不静默空节）。
func schemaSection(raw, ontoID, resolution string, version int) (*SchemaSection, error) {
	sp := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), sp); err != nil {
		return nil, fmt.Errorf("闭包预览 schema 解析失败（本体 %s）: %w", ontoID, err)
	}
	return &SchemaSection{
		OntoID:         ontoID,
		Resolution:     resolution,
		Version:        version,
		Concepts:       sp.Concepts,
		Relations:      sp.Relations,
		DataProperties: sp.DataProperties,
	}, nil
}

// danglingCodeText 悬空码人话（与运行面 danglingText 同文案口径）。
func danglingCodeText(code string) string {
	switch code {
	case "missing_ontology":
		return "本体已删除"
	case "missing_concept":
		return "概念不存在"
	}
	return "悬空（" + code + "）"
}

// findClosureCycles 彩色 DFS 回边环（与运行面 manager.findImportCycles 同构：至多 10 条、
// 路径签名去重；环语义=并集装载不受影响，报告仅为可观测性）。root=根本体。
func findClosureCycles(edges map[string][]string, root string) [][]string {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	cycles := [][]string{} // 空集=空数组非 null（REQ-541 清单形态纪律同源）
	seen := map[string]bool{}
	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		if len(cycles) >= 10 {
			return
		}
		color[node] = gray
		path = append(path, node)
		for _, tgt := range edges[node] {
			switch color[tgt] {
			case gray:
				start := 0
				for j, p := range path {
					if p == tgt {
						start = j
						break
					}
				}
				cyc := append(append([]string(nil), path[start:]...), tgt)
				sig := fmt.Sprint(cyc)
				if !seen[sig] {
					seen[sig] = true
					cycles = append(cycles, cyc)
				}
			case white:
				dfs(tgt, path)
			}
		}
		color[node] = black
	}
	if color[root] == white {
		dfs(root, nil)
	}
	return cycles
}
