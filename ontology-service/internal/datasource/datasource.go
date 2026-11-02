// Package datasource 由数据源构建本体（REQ-595/M406，第八构建路径）——
// NVIDIA AutoOntology Discover 生成段手法借鉴（99 号 v1.2 §9，借形态不引代码/栈，D-O15 不接 Python）。
// 管线五步：SQLite 只读内省 → LIMIT N 列画像采样（A1 证据注入/A6 熔断）→ 规则铺骨架
// （表→概念/非 FK 列→数据属性/声明 FK→对象属性/纯 FK junction→关系，确定性快路优先 A3）
// → LLM 窄调用补显示名/业务描述/隐含 FK 建议（温度随模型连接配置+严格消毒+确定性回退 A2/A4/A5）
// → 提案人工确认（前端 DraftTrimPanel 裁决形态，FK 建议必人审不静默落库）。
// 诚实边界（03 §2.74）：LLM 路径不产出公理（REQ-269）/FK 建议必须人审/采样只读
// （连接级 query_only 强制）/凭据不落库（提案结果不含连接串）/「自动化的是起草，不消失的是治理」。
//
// 一期仅 SQLite（modernc 纯 Go 驱动，ontology-service 既有依赖零新增）；
// MySQL/Postgres 需新驱动依赖留二期触发驱动。
package datasource

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

// Progress 进度回调（msg=人话阶段描述，永不虚构百分比——沿 REQ-295③ 口径）。
type Progress func(msg string)

// FKSuggestion LLM 隐含外键建议（结构性建议，必须人工裁决后前端才并入 spec——不静默落库）。
type FKSuggestion struct {
	Table       string `json:"table"`
	Column      string `json:"column"`
	TargetTable string `json:"target_table"`
	Reason      string `json:"reason,omitempty"`
}

// Proposal 分析提案（job 结果透出面）。
type Proposal struct {
	Name          string         `json:"name"`                   // 建议本体名（默认=SQLite 文件名去扩展名，前端可改）
	Spec          *pkgspec.Spec  `json:"spec"`                   // 确定性骨架（含声明 FK；LLM FK 建议不含在内）
	Tables        []TableProfile `json:"tables"`                 // 列画像（采样面透出）
	FKSuggestions []FKSuggestion `json:"fk_suggestions"`         // LLM 隐含 FK 建议（空=无或未启用 LLM）
	Warnings      []string       `json:"warnings,omitempty"`     // 熔断/截断/歧义/回退如实注记
	LLMUsed       bool           `json:"llm_used"`               // LLM 窄调用是否参与（false=纯确定性骨架）
	LLMSkipped    string         `json:"llm_skipped,omitempty"`  // LLM 未参与原因（人话）
	LLMFallback   bool           `json:"llm_fallback,omitempty"` // LLM 调用失败整体回退确定性结果（批不中断，A2）
}

// Options 分析参数。
type Options struct {
	Path       string // SQLite 数据库文件路径（服务端本地；只读打开）
	Name       string // 建议本体名（空=文件名去扩展名）
	SampleRows int    // 每表采样行数上限（默认 1000，env DATASOURCE_SAMPLE_ROWS；上限 5000）
	MaxTables  int    // 内省表数上限（默认 200，env DATASOURCE_MAX_TABLES）
}

// Analyze 管线编排入口：内省→画像→骨架→（可选）LLM 窄调用。
// creator 为 nil 时纯确定性骨架直出（LLM 未配置合法路径，LLMUsed=false 如实标注）。
func Analyze(ctx context.Context, opts Options, creator *llmcreate.Creator, onProgress Progress) (*Proposal, error) {
	notify := func(msg string) {
		if onProgress != nil {
			onProgress(msg)
		}
	}
	if strings.TrimSpace(opts.Path) == "" {
		return nil, fmt.Errorf("数据库文件路径必填")
	}
	sampleRows, maxTables := resolveLimits(opts)

	notify("连接数据库（只读）…")
	db, err := openReadOnly(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("无法以只读方式打开 SQLite 文件：%v", err)
	}
	defer db.Close()

	notify("内省表结构…")
	schema, err := introspect(ctx, db, maxTables)
	if err != nil {
		return nil, err
	}
	if len(schema.Tables) == 0 {
		return nil, fmt.Errorf("未发现任何数据表——请确认文件是 SQLite 数据库且包含用户表")
	}

	notify(fmt.Sprintf("列画像采样（%d 表 × ≤%d 行）…", len(schema.Tables), sampleRows))
	profiles, pwarnings := profileSchema(ctx, db, schema, sampleRows)

	notify("规则铺骨架（表→概念/列→属性/FK→关系）…")
	sk := buildSkeleton(schema, profiles)

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		base := filepath.Base(opts.Path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
		if name == "" || name == "." || name == "/" || name == "\\" {
			name = "数据源本体"
		}
	}
	sk.Spec.Name = name
	sk.Spec.Description = fmt.Sprintf("由数据源构建（SQLite 只读内省）：%d 表推导——概念=数据表、数据属性=非主键非外键列、对象关系=声明外键与纯关联表；LLM 补显示名与业务描述。", len(schema.Tables))
	// 骨架由构造保证合法；Validate 兜底防线（不合法即失败不静默放行——铁律）
	if errs := sk.Spec.Validate(); len(errs) > 0 {
		return nil, fmt.Errorf("骨架自校验未通过（%d 处）：%s", len(errs), errs[0].Error())
	}

	prop := &Proposal{
		Name:     name,
		Spec:     sk.Spec,
		Tables:   profiles,
		Warnings: append(append([]string{}, pwarnings...), sk.Warnings...),
	}

	if creator == nil {
		prop.LLMUsed = false
		prop.LLMSkipped = "LLM 未配置（PLATFORM_URL）——确定性骨架直出（显示名/描述留空可后补）"
		notify("LLM 未配置，确定性骨架直出")
		return prop, nil
	}

	notify("LLM 窄调用补显示名/描述/隐含 FK 建议…")
	res := refine(ctx, creator, schema, profiles, sk, prop.Warnings)
	prop.LLMUsed = res.Used
	prop.LLMSkipped = res.Skipped
	prop.LLMFallback = res.Fallback
	prop.FKSuggestions = res.FKSuggestions
	prop.Warnings = res.Warnings
	notify("分析完成")
	return prop, nil
}
