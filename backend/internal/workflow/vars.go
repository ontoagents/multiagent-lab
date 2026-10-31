package workflow

import (
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// 变量表 v1（REQ-562/M369 WF-① 地基——109 号 §WF-①「变量表 v1 优先复用 checkpoint 表载荷」：
// 无新表；VarTable JSON 可序列化随 checkpoint gob 走，WF-② Runner 注入消费；变量注入纳入
// REQ-531 复检的代码位在 Runner 注入后〔WF-② 交付项〕，本层只管 size 双限）。
// ---------------------------------------------------------------------------

// VarValue 单变量值（含溯源——写入节点与时刻；审计口径沿过程账本行级溯源先例）。
type VarValue struct {
	Value    json.RawMessage `json:"value"` // JSON 形态（type 由 Spec.Vars 声明约束）
	Type     string          `json:"type"`
	WrittenBy string         `json:"written_by"` // 写入节点 id
	WrittenAt string         `json:"written_at"`
}

// VarTable 变量表（run 内会话值载体——checkpoint 载荷形态）。
type VarTable struct {
	Vals map[string]VarValue `json:"vals"`
	// TotalBytes 全表字节总量账（含 key/元数据——总量 cap 判定基线）
	TotalBytes int `json:"total_bytes"`
	// Notes 截断注记清单（验收②：超单变量 cap/总量 cap 返回截断注记并落 run.warning——
	// Runner 侧把 Notes 转发为 run.warning；本层只累积）
	Notes []string `json:"notes,omitempty"`
}

// NewVarTable 空 表。
func NewVarTable() *VarTable {
	return &VarTable{Vals: map[string]VarValue{}}
}

// Set 写变量（spec 类型约束+单变量 cap+总量 cap；超限截断并出注记——验收②）。
// 返回截断注记（空=无截断）。未声明变量返回错误（写悬空在 Validate 已拦——运行期防御性再拦）。
func (t *VarTable) Set(spec *Spec, name, nodeID string, value any) (string, error) {
	var declared *VarSpec
	for i := range spec.Vars {
		if spec.Vars[i].Name == name {
			declared = &spec.Vars[i]
			break
		}
	}
	if declared == nil {
		return "", fmt.Errorf("写未声明变量 %q——先在 vars 声明", name)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("变量 %s 序列化失败: %w", name, err)
	}
	maxSize := declared.MaxSize
	if maxSize <= 0 {
		maxSize = VarMaxSize
	}
	note := ""
	if len(raw) > maxSize {
		// 截断（JSON 语义下按字节安全截断到合法 UTF-8 边界的近似——raw 尾部回退至有效边界）
		cut := truncateRaw(raw, maxSize)
		raw = cut
		note = fmt.Sprintf("变量 %s 写入超单变量上限（%d > %d 字节）——已截断（节点 %s）", name, len(raw), maxSize, nodeID)
		t.Note(note)
	}
	newTotal := t.TotalBytes + len(raw) + len(name) + 64
	if old, ok := t.Vals[name]; ok {
		newTotal -= len(old.Value) + len(name) + 64
	}
	if newTotal > VarTotalMaxSize {
		return fmt.Sprintf("变量 %s 写入被拒（变量表总量将超上限 %d 字节）——节点 %s（可提高 MaxSize 声明或精简上游产出）", name, VarTotalMaxSize, nodeID), fmt.Errorf("变量表总量超上限（%d > %d）", newTotal, VarTotalMaxSize)
	}
	t.Vals[name] = VarValue{Value: raw, Type: declared.Type, WrittenBy: nodeID, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
	t.TotalBytes = newTotal
	return note, nil
}

// Get 读变量（原始 JSON 形态——Runner 注入消费）。
func (t *VarTable) Get(name string) (json.RawMessage, bool) {
	if t == nil || t.Vals == nil {
		return nil, false
	}
	v, ok := t.Vals[name]
	return v.Value, ok
}

// Note 追加截断注记（去重）。
func (t *VarTable) Note(note string) {
	for _, n := range t.Notes {
		if n == note {
			return
		}
	}
	t.Notes = append(t.Notes, note)
}

// truncateRaw JSON 字节安全截断（回退到最近的合法边界——值形态保真尽力，截断注记如实标注）。
func truncateRaw(raw []byte, max int) []byte {
	if max <= 0 || len(raw) <= max {
		return raw
	}
	cut := raw[:max]
	// 回退 UTF-8 多字节截断（尾字节连续 10xxxxxx 计数）
	for i := 0; i < 4 && len(cut)-i-1 > 0; i++ {
		b := cut[len(cut)-1-i]
		if b&0xC0 != 0x80 { // 非 continuation 字节
			if b >= 0xF0 && i < 3 {
				cut = cut[:len(cut)-1-i] // 多字节首字节被截断——回退
			}
			break
		}
	}
	return cut
}
