package datasource

// 列画像采样（REQ-595 管线步②，99 号 v1.2 §9.1 步 2 手法借鉴）：
// SELECT * LIMIT N 采样 → 每列 top-5 高频值/采样内唯一性/低基数枚举候选/日期格式推断/空值占比。
// A6 采样熔断：连续 3 表采样基础设施失败→本次跳过其余表采样，骨架继续（声明类型兜底）；
// 单表失败即跳过该表（warning 注记），批不中断。

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ValueCount 高频值计数。
type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// ColumnProfile 列画像（采样面透出；is_unique=采样内口径，如实不夸大）。
type ColumnProfile struct {
	Column         string       `json:"column"`
	DeclType       string       `json:"decl_type"`
	SampledRows    int          `json:"sampled_rows"`
	NullCount      int          `json:"null_count"`
	IsUnique       bool         `json:"is_unique"`
	TopValues      []ValueCount `json:"top_values,omitempty"`
	EnumCandidates []string     `json:"enum_candidates,omitempty"` // 采样内 distinct ≤25 全量枚举
	DateFormats    []string     `json:"date_formats,omitempty"`    // 采样值日期格式推断
}

// TableProfile 表画像（Error 非空=该表采样失败已跳过）。
type TableProfile struct {
	Table       string          `json:"table"`
	SampledRows int             `json:"sampled_rows"`
	Columns     []ColumnProfile `json:"columns,omitempty"`
	Error       string          `json:"error,omitempty"`
}

const (
	// defaultSampleRows 默认每表采样行数（沿 99 号 v1.2 `SELECT * LIMIT 1000` 口径）。
	defaultSampleRows = 1000
	// maxSampleRows 硬上限（护栏）。
	maxSampleRows = 5000
	// defaultMaxTables 内省表数默认上限。
	defaultMaxTables = 200
	// enumLimit 低基数枚举候选阈值（采样内 distinct ≤25 取全量，沿 99 号口径）。
	enumLimit = 25
	// topK top 高频值条数。
	topK = 5
	// breakerLimit A6 采样熔断阈值：连续 3 表基础设施失败→跳过其余表采样。
	breakerLimit = 3
)

// resolveLimits 采样参数归一（env 可调：DATASOURCE_SAMPLE_ROWS / DATASOURCE_MAX_TABLES；
// 上限硬护栏防误配打爆内存——单机自部署自担口径的边界内再兜一层）。
func resolveLimits(opts Options) (sampleRows, maxTables int) {
	sampleRows = defaultSampleRows
	if v := os.Getenv("DATASOURCE_SAMPLE_ROWS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sampleRows = n
		}
	}
	if sampleRows > maxSampleRows {
		sampleRows = maxSampleRows
	}
	if opts.SampleRows > 0 {
		sampleRows = opts.SampleRows
		if sampleRows > maxSampleRows {
			sampleRows = maxSampleRows
		}
	}
	maxTables = defaultMaxTables
	if v := os.Getenv("DATASOURCE_MAX_TABLES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTables = n
		}
	}
	if opts.MaxTables > 0 {
		maxTables = opts.MaxTables
	}
	return sampleRows, maxTables
}

// quoteIdent 标识符双引号转义（SELECT 列/表用）。
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// profileSchema 逐表采样画像（返回画像与 warning 注记）。
func profileSchema(ctx context.Context, db *sql.DB, schema *Schema, sampleRows int) ([]TableProfile, []string) {
	profiles := make([]TableProfile, 0, len(schema.Tables))
	var warnings []string
	consecFail := 0 // A6 熔断计数（连续基础设施失败）
	broken := false
	for _, td := range schema.Tables {
		if broken {
			profiles = append(profiles, TableProfile{Table: td.Name, Error: "采样熔断：连续失败跳过（骨架继续，按声明类型推导）"})
			continue
		}
		tp := TableProfile{Table: td.Name}
		err := profileTable(ctx, db, td, sampleRows, &tp)
		if err != nil {
			consecFail++
			tp.Error = err.Error()
			profiles = append(profiles, tp)
			warnings = append(warnings, fmt.Sprintf("表「%s」采样失败已跳过：%v", td.Name, err))
			if consecFail >= breakerLimit {
				broken = true
				warnings = append(warnings, "采样熔断（连续 3 表失败）：其余表跳过采样，骨架按声明类型推导继续")
			}
			continue
		}
		consecFail = 0
		profiles = append(profiles, tp)
	}
	return profiles, warnings
}

// profileTable 单表采样画像（失败返回 error，不中断批）。
func profileTable(ctx context.Context, db *sql.DB, td TableDef, sampleRows int, tp *TableProfile) error {
	if len(td.Columns) == 0 {
		tp.Error = "无列（跳过采样）"
		return fmt.Errorf("无列")
	}
	colNames := make([]string, len(td.Columns))
	for i, c := range td.Columns {
		colNames[i] = quoteIdent(c.Name)
	}
	q := fmt.Sprintf("SELECT %s FROM %s LIMIT %d", strings.Join(colNames, ", "), quoteIdent(td.Name), sampleRows)
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return fmt.Errorf("采样查询失败：%v", err)
	}
	defer rows.Close()

	// 每列累加器
	type acc struct {
		vals     map[string]int
		nulls    int
		distinct map[string]struct{}
	}
	accs := make([]*acc, len(td.Columns))
	for i := range accs {
		accs[i] = &acc{vals: map[string]int{}, distinct: map[string]struct{}{}}
	}
	total := 0
	for rows.Next() {
		dest := make([]sql.NullString, len(td.Columns))
		destPtrs := make([]any, len(td.Columns))
		for i := range dest {
			destPtrs[i] = &dest[i]
		}
		if err := rows.Scan(destPtrs...); err != nil {
			return fmt.Errorf("采样行读取失败：%v", err)
		}
		total++
		for i := range td.Columns {
			if !dest[i].Valid || dest[i].String == "" {
				accs[i].nulls++
				continue
			}
			v := dest[i].String
			if len(v) > 200 {
				v = v[:200] // 长文本截断计数（画像展示面不吞内存）
			}
			accs[i].vals[v]++
			accs[i].distinct[v] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("采样遍历失败：%v", err)
	}
	tp.SampledRows = total
	tp.Columns = make([]ColumnProfile, len(td.Columns))
	for i, c := range td.Columns {
		cp := ColumnProfile{Column: c.Name, DeclType: c.DeclType, SampledRows: total, NullCount: accs[i].nulls}
		nonNull := total - accs[i].nulls
		cp.IsUnique = nonNull > 0 && len(accs[i].distinct) == nonNull
		// top-k 高频值
		type kv struct {
			k string
			v int
		}
		lst := make([]kv, 0, len(accs[i].vals))
		for k, v := range accs[i].vals {
			lst = append(lst, kv{k, v})
		}
		sort.Slice(lst, func(a, b int) bool {
			if lst[a].v != lst[b].v {
				return lst[a].v > lst[b].v
			}
			return lst[a].k < lst[b].k
		})
		for j, e := range lst {
			if j >= topK {
				break
			}
			cp.TopValues = append(cp.TopValues, ValueCount{Value: e.k, Count: e.v})
		}
		// 低基数枚举候选（采样内 distinct ≤25 取全量）
		if len(accs[i].distinct) > 0 && len(accs[i].distinct) <= enumLimit {
			enum := make([]string, 0, len(accs[i].distinct))
			for k := range accs[i].distinct {
				enum = append(enum, k)
			}
			sort.Strings(enum)
			cp.EnumCandidates = enum
		}
		// 日期格式推断（仅字符串值）
		cp.DateFormats = inferDateFormats(accs[i].vals)
		tp.Columns[i] = cp
	}
	return nil
}

// dateLayouts 日期格式推断表（Go layout → 展示标签）。
var dateLayouts = []struct{ layout, label string }{
	{"2006-01-02", "YYYY-MM-DD"},
	{"2006-01-02 15:04:05", "YYYY-MM-DD HH:MM:SS"},
	{"2006-01-02T15:04:05", "ISO8601"},
	{"2006-01-02T15:04:05Z07:00", "ISO8601 带时区"},
	{"2006/01/02", "YYYY/MM/DD"},
	{"2006年01月02日", "YYYY年MM月DD日"},
	{"02/01/2006", "DD/MM/YYYY"},
	{"01/02/2006", "MM/DD/YYYY"},
	{"02012006", "DDMMYYYY"},
}

// inferDateFormats 采样值日期格式推断（≥60% 非空值命中同一格式才报告——偶发日期样字符串不算）。
// D/M 与 M/D 歧义如实标注（沿 99 号「混合格式显式告警」口径）。
func inferDateFormats(vals map[string]int) []string {
	if len(vals) == 0 {
		return nil
	}
	hits := map[string]int{}
	total := 0
	for v, n := range vals {
		total += n
		if lbl := matchDateFormat(v); lbl != "" {
			hits[lbl] += n
		}
	}
	if total == 0 {
		return nil
	}
	var out []string
	for lbl, n := range hits {
		if n*100 >= total*60 { // ≥60% 值命中该格式
			out = append(out, fmt.Sprintf("%s（%d/%d 值）", lbl, n, total))
		}
	}
	if len(out) > 1 {
		// 混合格式显式告警（99 号手法）
		out = append(out, "混合格式——建议人工核对")
	}
	sort.Strings(out)
	return out
}

// isAllDigits 纯数字判定（Unix 时间戳启发前置）。
func isAllDigits(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// matchDateFormat 单值格式判定（含 Unix 时间戳数值段启发）。
func matchDateFormat(v string) string {
	// 纯数字 → Unix 时间戳启发（秒 2001~2100 / 毫秒段）
	if isAllDigits(v) {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			switch {
			case n >= 1000000000 && n <= 4102444800:
				return "Unix 时间戳（秒）"
			case n >= 1000000000000 && n <= 4102444800000:
				return "Unix 时间戳（毫秒）"
			}
		}
		return ""
	}
	if len(v) < 6 || len(v) > 32 {
		return ""
	}
	for _, dl := range dateLayouts {
		if len([]rune(dl.layout)) != len(v) && dl.layout != "2006-01-02T15:04:05Z07:00" {
			continue
		}
		if _, err := time.ParseInLocation(dl.layout, v, time.UTC); err == nil {
			// D/M 与 M/D 歧义消解：两部分 >12 可裁决
			if dl.label == "DD/MM/YYYY" || dl.label == "MM/DD/YYYY" {
				parts := strings.Split(v, "/")
				if len(parts) == 3 {
					a, e1 := strconv.Atoi(parts[0])
					b, e2 := strconv.Atoi(parts[1])
					if e1 == nil && e2 == nil {
						switch {
						case a > 12:
							return "DD/MM/YYYY"
						case b > 12:
							return "MM/DD/YYYY"
						}
					}
				}
				return dl.label // ≤12 双可：如实报告（歧义由 inferDateFormats 混合格式注记兜底）
			}
			return dl.label
		}
	}
	return ""
}
