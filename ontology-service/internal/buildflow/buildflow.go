// Package buildflow 构建管线编排层（REQ-340 A2，80 号建议项 Z/A2）。
//
// 六段管线模型：来源（source）→ 抽取（extract）→ 草稿（draft）→ 质量门禁（gate）→ 审查（review）→ 入库（ingest）。
// 三条 LLM 构建路径收敛为参数化实例（registry）：
//   - ai-draft：S1 AI 创建（rest_async.go，llmcreate 生成-校验循环内聚 extract/draft/gate；审查=前端确认卡；入库=通用 create+saveSpec）
//   - ontoextend-draft：OntoExtend 扩展（rest_build.go ontoextendDraft，ODP 片段即 draft；审查/入库=通用 merge/preview+apply，REQ-157 底座）
//   - kb-extract：由知识库构建（backend ontobuild 编排，经 POST /api/buildflow/traces 回传段留痕——跨服务路径的聚合口）
//
// 段边界单点留痕：每段一条 SegmentTrace（Tracer 出口）。REQ-328 零丢失对账挂管线段边界
// 一处覆盖三路径——对账埋点只需实现 Tracer 接管（本包不发持久化，零行为变更）。
// 纯观测件：Run 只顺序执行段并计时留痕，不改写任何路径的既有语义。
package buildflow

import (
	"context"
	"log"
	"sync"
	"time"
)

// Stage 六段管线阶段。
type Stage string

const (
	StageSource  Stage = "source"  // 来源：领域描述 / ODP 模式 / KB 语料
	StageExtract Stage = "extract" // 抽取：LLM 抽取 / KG 直转 / 片段装配
	StageDraft   Stage = "draft"   // 草稿：spec_json 形态产出（含校验循环）
	StageGate    Stage = "gate"    // 质量门禁：qualitygate 15 项（strict 门禁在入库侧另有语义）
	StageReview  Stage = "review"  // 审查：人工确认（前端确认卡 / merge preview）
	StageIngest  Stage = "ingest"  // 入库：create+saveSpec / merge apply（版本快照）
)

// StageOrder 六段全序（实例可只覆盖子集——如 ai-draft 的审查段在前端确认步）。
var StageOrder = []Stage{StageSource, StageExtract, StageDraft, StageGate, StageReview, StageIngest}

// 管线实例注册表：路径名 → 覆盖段（编排事实单源；新路径在此登记）。
var registry = map[string][]Stage{
	"ai-draft":         {StageSource, StageExtract, StageDraft, StageGate, StageReview, StageIngest},
	"ontoextend-draft": {StageSource, StageExtract, StageDraft, StageGate, StageReview, StageIngest},
	"kb-extract":       {StageSource, StageExtract, StageDraft, StageGate, StageReview, StageIngest},
	// REQ-343/OE-1：CQ 驱动增量扩展（ontorag 三段式 RAG）——extract=元素抽取+索引检索、
	// draft=逐 CQ 生成+两阶段校验；审查/入库=通用 merge/preview+apply（REQ-157 底座，不在本端点）。
	"cq-rag-draft": {StageSource, StageExtract, StageDraft, StageGate, StageReview, StageIngest},
}

// Registered 已登记的管线实例名（诊断/测试用）。
func Registered() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

// Covers 管线实例是否声明覆盖某段。
func Covers(pipeline string, st Stage) bool {
	for _, s := range registry[pipeline] {
		if s == st {
			return true
		}
	}
	return false
}

// SegmentTrace 段边界留痕（REQ-328 对账挂点：一段一记录）。
type SegmentTrace struct {
	Pipeline   string    `json:"pipeline"`              // ai-draft | ontoextend-draft | kb-extract
	Stage      Stage     `json:"stage"`                 // source | extract | draft | gate | review | ingest
	Status     string    `json:"status"`                // ok | fail | skip
	StartedAt  time.Time `json:"started_at"`            // 段起始（UTC）
	DurationMS int64     `json:"duration_ms"`           // 段耗时
	Note       string    `json:"note,omitempty"`        // 段摘要（来源描述/轮次/质量分/错误——人话，短）
	OntologyID string    `json:"ontology_id,omitempty"` // 入库段落点（可得时）
}

// Segment 单段执行体：Run 返回人话摘要 note；Skip=true 只留 skip 痕不执行。
type Segment struct {
	Stage Stage
	Skip  bool
	Run   func(ctx context.Context) (note string, err error)
}

// Tracer 段留痕出口（默认 LogTracer；REQ-328 可接管为持久化实现）。
type Tracer interface {
	Emit(SegmentTrace)
}

// LogTracer 结构化日志出口（单机默认；一行一段，可 grep）。
type LogTracer struct{}

// Emit 落日志。
func (LogTracer) Emit(t SegmentTrace) {
	log.Printf("[buildflow] pipeline=%s stage=%s status=%s dur=%dms ontology=%s note=%s",
		t.Pipeline, t.Stage, t.Status, t.DurationMS, t.OntologyID, t.Note)
}

var (
	tracerMu sync.RWMutex
	tracer   Tracer = LogTracer{}
)

// SetTracer 接管段留痕出口（nil 恢复默认；REQ-328 对账持久化的挂点）。
func SetTracer(t Tracer) {
	tracerMu.Lock()
	defer tracerMu.Unlock()
	if t == nil {
		t = LogTracer{}
	}
	tracer = t
}

func emit(t SegmentTrace) {
	tracerMu.RLock()
	defer tracerMu.RUnlock()
	tracer.Emit(t)
}

// Run 顺序执行段并逐段留痕；首个错误中断（后续段不再执行、不留痕——失败段已带错误留痕）。
// 纯观测约定：各路径既有控制流不强制改经 Run——单段场景用 Emit 拍段（见 Emit）。
func Run(ctx context.Context, pipeline string, segments ...Segment) error {
	for _, seg := range segments {
		if !Covers(pipeline, seg.Stage) {
			continue // 未声明段不执行不留痕（编排事实以 registry 为准）
		}
		if seg.Skip {
			emit(SegmentTrace{Pipeline: pipeline, Stage: seg.Stage, Status: "skip", StartedAt: time.Now().UTC()})
			continue
		}
		started := time.Now()
		note, err := seg.Run(ctx)
		st := "ok"
		if err != nil {
			st = "fail"
			if note == "" {
				note = err.Error()
			}
		}
		emit(SegmentTrace{
			Pipeline: pipeline, Stage: seg.Stage, Status: st,
			StartedAt: started.UTC(), DurationMS: time.Since(started).Milliseconds(), Note: note,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Emit 拍一段（单段场景：路径控制流不便整体改经 Run 时，在段边界直接留痕）。
func Emit(pipeline string, st Stage, started time.Time, status, note, ontologyID string) {
	if status == "" {
		status = "ok"
	}
	emit(SegmentTrace{
		Pipeline: pipeline, Stage: st, Status: status,
		StartedAt: started.UTC(), DurationMS: time.Since(started).Milliseconds(),
		Note: note, OntologyID: ontologyID,
	})
}

// EmitAt 直发一条已成形留痕（远端路径经聚合端点回传时段计时已在调用方完成，原样透传）。
func EmitAt(t SegmentTrace) {
	if t.Status == "" {
		t.Status = "ok"
	}
	emit(t)
}
