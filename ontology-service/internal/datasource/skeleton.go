package datasource

// 规则铺骨架（REQ-595 管线步③，确定性快路优先——99 号 v1.2 手法 A3）：
// 表→概念 / 非主键非外键列→数据属性 / 声明 FK→对象属性（Evidence=declared）/ 纯 FK junction→关系。
// 命名确定性：sanitize→碰撞限定→防撞后缀；LLM 只补显示名与描述（llmrefine.go），不改结构名。

import (
	"fmt"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"
)

// skeletonBuildResult 骨架构建结果。
type skeletonBuildResult struct {
	Spec     *pkgspec.Spec
	Warnings []string
}

// buildSkeleton 确定性骨架（输入=内省 schema+列画像；纯结构规则，零 LLM）。
func buildSkeleton(schema *Schema, profiles []TableProfile) *skeletonBuildResult {
	res := &skeletonBuildResult{Spec: &pkgspec.Spec{
		Concepts: []pkgspec.Concept{}, Relations: []pkgspec.Relation{}, Instances: []pkgspec.Instance{}},
	}
	if schema.TruncatedTables > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("表数超内省上限，仅处理前 %d 表（其余 %d 表未纳入——可用 DATASOURCE_MAX_TABLES 调大）", len(schema.Tables), schema.TruncatedTables))
	}
	// junction 判定：≥2 声明 FK 且所有列均为 PK 或 FK 列（无载荷列）→ 纯关联表，不出概念
	junction := map[string]bool{}
	fkByCol := map[string]map[string]FKDef{} // table → column → fk
	for i := range schema.Tables {
		td := &schema.Tables[i]
		colFK := map[string]FKDef{}
		for _, fk := range td.FKs {
			colFK[fk.Column] = fk
		}
		fkByCol[td.Name] = colFK
		if len(td.FKs) >= 2 {
			pure := true
			for _, c := range td.Columns {
				if _, isFK := colFK[c.Name]; !isFK && !c.IsPK {
					pure = false
					break
				}
			}
			if pure && len(td.Columns) > 0 {
				junction[td.Name] = true
			}
		}
	}

	// 第一步：非 junction 表 → 概念（先铺概念，关系引用才有目标）
	conceptNames := map[string]string{} // sanitize 名 → 原表名
	for i := range schema.Tables {
		td := &schema.Tables[i]
		if junction[td.Name] {
			continue
		}
		base := sanitizeName(td.Name)
		cn := uniqueName(base, td.Name+" "+base, conceptNames)
		conceptNames[cn] = td.Name
		res.Spec.Concepts = append(res.Spec.Concepts, pkgspec.Concept{
			Name: cn, Label: td.Name, // Label=原始表名；LLM 窄调用可覆盖为中文显示名
		})
	}
	// 表被截断/引用缺失的悬空 FK 防线：目标表不在概念集（junction 或截断）→ 关系跳过并注记
	touched := map[string]string{} // 概念名占用（防概念与关系重名——spec 关系名独立命名空间，但为可读性防撞）

	// 第二步：声明 FK → 对象属性 / junction → 两两关系
	for i := range schema.Tables {
		td := &schema.Tables[i]
		if junction[td.Name] {
			// 纯关联表 → 取前两个 FK 目标两两建关系（>2 个 FK 如实注记）
			if len(td.FKs) >= 2 {
				a, b := td.FKs[0], td.FKs[1]
				fromC := conceptNameOf(a.RefTable, conceptNames)
				toC := conceptNameOf(b.RefTable, conceptNames)
				if fromC != "" && toC != "" {
					base := sanitizeName(td.Name)
					rn := uniqueName(base, td.Name+" "+base, touched)
					res.Spec.Relations = append(res.Spec.Relations, pkgspec.Relation{
						Name: rn, Label: td.Name, From: fromC, To: toC, Evidence: "declared",
						Definition: fmt.Sprintf("由纯外键关联表「%s」推导（junction）", td.Name),
					})
					if len(td.FKs) > 2 {
						res.Warnings = append(res.Warnings, fmt.Sprintf("关联表「%s」有 %d 个外键列，仅按前两个建关系（其余未建——可人工补充）", td.Name, len(td.FKs)))
					}
				} else {
					res.Warnings = append(res.Warnings, fmt.Sprintf("关联表「%s」外键目标表未纳入（截断或引用缺失），未建关系", td.Name))
				}
			}
			continue
		}
		self := conceptNameOf(td.Name, conceptNames)
		colFK := fkByCol[td.Name]
		for _, fk := range td.FKs {
			target := conceptNameOf(fk.RefTable, conceptNames)
			if target == "" {
				res.Warnings = append(res.Warnings, fmt.Sprintf("表「%s」外键列「%s」目标表「%s」未纳入，未建关系", td.Name, fk.Column, fk.RefTable))
				continue
			}
			// 关系名：优先 FK 列名；被占则表限定（sanitizeName 恒非空，uniqueName 恒有解）
			base := sanitizeName(fk.Column)
			rn := uniqueName(base, sanitizeName(td.Name)+" "+base, touched)
			res.Spec.Relations = append(res.Spec.Relations, pkgspec.Relation{
				Name: rn, Label: fk.Column, From: self, To: target, Evidence: "declared",
				Definition: fmt.Sprintf("由声明外键「%s.%s → %s」推导", td.Name, fk.Column, fk.RefTable),
			})
		}
		// 第三步：非主键非外键列 → 数据属性（range 按声明类型）
		for _, c := range td.Columns {
			if _, isFK := colFK[c.Name]; isFK || c.IsPK {
				continue
			}
			dpNames := dataPropNameSet(res.Spec)
			cb, tb := sanitizeName(c.Name), sanitizeName(td.Name)
			pn := uniqueName(cb, tb+"_"+cb, dpNames)
			res.Spec.DataProperties = append(res.Spec.DataProperties, pkgspec.DataProperty{
				Name: pn, Label: c.Name, Domain: self, Range: inferRange(c.DeclType),
			})
		}
	}
	return res
}

// dataPropNameSet 收集已占用数据属性名（防撞）。
func dataPropNameSet(sp *pkgspec.Spec) map[string]string {
	m := map[string]string{}
	for _, dp := range sp.DataProperties {
		m[dp.Name] = dp.Name
	}
	return m
}

// conceptNameOf 目标表 → 概念名（junction 表/截断表不在集内返回空）。
func conceptNameOf(table string, conceptNames map[string]string) string {
	for cn, src := range conceptNames {
		if src == table {
			return cn
		}
	}
	return ""
}

// sanitizeName 确定性命名清理：去首尾空白+内部空白折叠；空则回退占位。
func sanitizeName(raw string) string {
	s := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if s == "" {
		s = "未命名"
	}
	return s
}

// uniqueName 防撞：base 可用直取；占用则用 qualifiedBase 限定；仍撞加数字后缀。
// （spec 校验口径=概念/关系/数据属性各自命名空间内唯一——限定名按调用点定制，
// 数据属性=<表>_<列>，概念/关系=原表名前缀。）
func uniqueName(base, qualifiedBase string, taken map[string]string) string {
	if base == "" {
		return ""
	}
	if _, ok := taken[base]; !ok {
		taken[base] = base
		return base
	}
	if qualifiedBase != "" && qualifiedBase != base {
		if _, ok := taken[qualifiedBase]; !ok {
			taken[qualifiedBase] = qualifiedBase
			return qualifiedBase
		}
	}
	pfx := qualifiedBase
	if pfx == "" || pfx == base {
		pfx = base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s %d", pfx, i)
		if _, ok := taken[cand]; !ok {
			taken[cand] = cand
			return cand
		}
	}
}

// inferRange 声明类型 → 数据属性 range 短名（SQLite 动态类型下以声明类型为确定性依据；
// 采样画像的日期格式/枚举证据展示给人核对，不参与 range 自动改判）。
func inferRange(declType string) string {
	t := strings.ToUpper(declType)
	switch {
	case strings.Contains(t, "INT"):
		return "integer"
	case strings.Contains(t, "BOOL"):
		return "boolean"
	case strings.Contains(t, "REAL"), strings.Contains(t, "FLOA"), strings.Contains(t, "DOUB"),
		strings.Contains(t, "DEC"), strings.Contains(t, "NUM"):
		return "number"
	case strings.Contains(t, "DATE"), strings.Contains(t, "TIME"):
		return "date"
	default: // TEXT/CHAR/CLOB/BLOB/空（SQLite 动态类型）→ string
		return "string"
	}
}
