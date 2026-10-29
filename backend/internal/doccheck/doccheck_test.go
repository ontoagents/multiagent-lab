// Package doccheck REQ-399/M195：docs 大档一致性校验器（常跑测试承载——沿 knowledge 校验器先例）。
// 三件核验（03 §2.59 详设口径）：
//  ①交叉引用存在性——docs 大档引用的 REQ-nnn 必须在 18 号注册表主表在册；
//  ②头部版本与迭代表一致性——01/02/03/18/20 五档头部状态行版本号必须在各自迭代表在行
//   （防「头部 bump 迭代表未落」的滚动漂移——03 头部 rebase 回退史/02 M117 滞后同类实证）；
//  ③18 注册表引用核对——18 迭代表各版本行号单调（同版本号重复出现=撞号，允许「撞号注」明示豁免）。
//
// 覆盖档=01/02/03/04/18/20；04 无迭代表仅参与①。
package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const repoRoot = "../../../"

var bigDocs = []string{
	"docs/01_智能体_需求文档_PRD.md",
	"docs/02_智能体_技术方案设计.md",
	"docs/03_本体_需求文档.md",
	"docs/04_本体_方案设计.md",
	"docs/18_REQ编号注册表.md",
	"docs/20_回归冒烟清单.md",
}

var headVerRe = regexp.MustCompile(`^> 状态：[^0-9]*v(\d+\.\d+)`)
var iterVerRe = regexp.MustCompile(`^\| (v\d+\.\d+) \|`)
var reqRefRe = regexp.MustCompile(`REQ-(\d{3})`)
var registryRowRe = regexp.MustCompile(`^\| REQ-(\d{3}) \|`)

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, rel))
	if err != nil {
		t.Fatalf("读取 %s: %v", rel, err)
	}
	return string(b)
}

// TestRegistryREQReferences ①：docs 大档中的 REQ-nnn 引用必须在 18 注册表主表在册。
func TestRegistryREQReferences(t *testing.T) {
	// REQ 在册来源=18 注册表主表 + 01/03 需求表（存量 REQ-60~119 行内登记于 01/03，
	// 18 为新号唯一权威不迁移——引用核对以三源并集为口径）
	known := map[string]bool{}
	// 01/03 需求行形态含注（REQ-311（v1.45...）/REQ-355（...）——行首 `| REQ-nnn` 宽放为
	// 「表格行内以 REQ-nnn 开头语义位」：`| REQ-` 前缀或 `｜ REQ-（` 变体均计入在册）
	reqRowRe := regexp.MustCompile(`^\|\s*REQ-(\d{3})`)
	for _, src := range []string{
		"docs/18_REQ编号注册表.md",
		"docs/01_智能体_需求文档_PRD.md",
		"docs/03_本体_需求文档.md",
		"docs/11_知识库_需求文档.md",  // KB 域 REQ 行（REQ-102/241 等在册处）
		"docs/12_知识库_方案设计.md",
	} {
		reg := readRepo(t, src)
		for _, line := range strings.Split(reg, "\n") {
			if m := reqRowRe.FindStringSubmatch(line); m != nil {
				known[m[1]] = true
			}
		}
	}
	if len(known) < 100 {
		t.Fatalf("注册表主表行解析异常（应 ≥100 行）: %d", len(known))
	}
	// 豁免清单（存量历史遗留引用——注册表行缺位，防复发目标=不新增；补登记随台账轮）
	legacyExempt := map[string]string{
		"102": "Git 视图深链（01 头注引用）——注册表行缺位历史遗留",
		"124": "折叠树（01 聚合行 REQ-01~06,REQ-124 引用）——无独立注册行",
		"244": "02 迭代表 M70 引用——注册行缺位待核对",
		"414": "01 v1.84 实用化批观察项（KB 富格式提级）引用——注册行随该批补登（并行批）",
	}
	for _, doc := range bigDocs {
		text := readRepo(t, doc)
		seen := map[string]bool{}
		var missing []string
		for _, m := range reqRefRe.FindAllStringSubmatch(text, -1) {
			id := m[1]
			if seen[id] || known[id] {
				seen[id] = true
				continue
			}
			seen[id] = true
			if reason, ok := legacyExempt[id]; ok {
				t.Logf("豁免（%s）：%s 引用 REQ-%s", reason, doc, id)
				continue
			}
			missing = append(missing, "REQ-"+id)
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("%s 引用了注册表不存在的 REQ：%s", doc, strings.Join(missing, "、"))
		}
	}
}

// TestHeadVersionMatchesIterationTable ②：五档头部版本号必须在其迭代表在行
// （头部 bump 而迭代表未落=滚动漂移——REQ-399 立项实证的机械化核验）。
func TestHeadVersionMatchesIterationTable(t *testing.T) {
	for _, doc := range []string{
		"docs/01_智能体_需求文档_PRD.md",
		"docs/02_智能体_技术方案设计.md",
		"docs/03_本体_需求文档.md",
		"docs/18_REQ编号注册表.md",
		"docs/20_回归冒烟清单.md",
	} {
		text := readRepo(t, doc)
		var head string
		for _, line := range strings.Split(text, "\n") {
			if m := headVerRe.FindStringSubmatch(line); m != nil {
				head = m[1]
				break
			}
		}
		if head == "" {
			t.Errorf("%s 头部状态行未解析出版本号", doc)
			continue
		}
		found := false
		for _, line := range strings.Split(text, "\n") {
			if m := iterVerRe.FindStringSubmatch(line); m != nil && m[1] == "v"+head { // iterVerRe 捕获含 v 前缀
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s 头部版本 v%s 未在迭代表在行（头部 bump 未落迭代表=滚动漂移）", doc, head)
		}
	}
}

// TestIterationRecentDupGuards ③：迭代表**顶部近期行**（前 6 行）同版本号重复=撞号
// （允许「撞号注」明示豁免——行内含「撞号」视为已如实登记）。范围收窄说明：全史撞号属
// 并行开发既成事实（批量补注无增量价值），防回归目标=近期窗口不再新增无注撞号
// （02 v0.284 双占为本检查立项实证）。
func TestIterationRecentDupGuards(t *testing.T) {
	const recent = 6
	for _, doc := range []string{
		"docs/01_智能体_需求文档_PRD.md",
		"docs/02_智能体_技术方案设计.md",
		"docs/03_本体_需求文档.md",
		"docs/18_REQ编号注册表.md",
		"docs/20_回归冒烟清单.md",
	} {
		text := readRepo(t, doc)
		var recentRows []string
		for _, line := range strings.Split(text, "\n") {
			if m := iterVerRe.FindStringSubmatch(line); m != nil {
				recentRows = append(recentRows, line)
				if len(recentRows) >= recent {
					break
				}
			}
		}
		counts := map[string]int{}
		notes := map[string]bool{}
		for _, line := range recentRows {
			v := iterVerRe.FindStringSubmatch(line)[1]
			counts[v]++
			if strings.Contains(line, "撞号") {
				notes[v] = true
			}
		}
		var dups []string
		for v, n := range counts {
			if n > 1 && !notes[v] {
				dups = append(dups, v)
			}
		}
		if len(dups) > 0 {
			sort.Strings(dups)
			t.Errorf("%s 迭代表近期 %d 行版本撞号且无撞号注：%s（补「撞号注」明示或顺延版本）", doc, recent, strings.Join(dups, "、"))
		}
	}
}

// TestHeadLineBudget ④（REQ-498 治理批）：五档头部「> 状态：」行 ≤2,000 字节——
// 「当前版本+一行摘要+前版指针」口径的机械守卫，禁止递归嵌套前版链
// （治理报告实测：压平前五档头部行合计 94,032B，单行最长 33,126B）。
func TestHeadLineBudget(t *testing.T) {
	for _, doc := range []string{
		"docs/01_智能体_需求文档_PRD.md",
		"docs/02_智能体_技术方案设计.md",
		"docs/03_本体_需求文档.md",
		"docs/18_REQ编号注册表.md",
		"docs/20_回归冒烟清单.md",
	} {
		text := readRepo(t, doc)
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "> 状态：") {
				if n := len(line); n > 2000 {
					t.Errorf("%s 头部状态行 %d 字节超 2,000B 上限（REQ-498 口径：当前版本+一行摘要+前版指针，前版链史归迭代表）", doc, n)
				}
				break
			}
		}
	}
}

// TestAgentsWindowEntryBudget ⑤（REQ-498 治理批）：AGENTS「当前状态」单条 ≤600 字节——
// 窗口条目「REQ 号+一句话状态+全量指针」一行口径的机械守卫
// （拍板=保 20 条压单条：36.9KB→约 5KB，详录全量在各档迭代表）。
func TestAgentsWindowEntryBudget(t *testing.T) {
	text := readRepo(t, "AGENTS.md")
	i := strings.Index(text, "## 当前状态")
	if i < 0 {
		t.Fatal("AGENTS.md 未找到「当前状态」节")
	}
	for _, line := range strings.Split(text[i:], "\n") {
		if strings.HasPrefix(line, "- **") {
			if n := len(line); n > 600 {
				t.Errorf("AGENTS 当前状态单条 %d 字节超 600B 上限（REQ-498 拍板：一行指针式，详录归各档迭代表）——条目头：%.40s", n, line)
			}
		}
	}
}

// TestReqIndexOpenSet ⑥（REQ-499 人读/机读分层）：docs/19 开放需求索引机读格式校验——
// 状态列枚举合法（📋/🚧/🔁/🧊，完结态 ✅/⛔ 禁止出现=完结即删行）+行内 REQ 号必须存在于 18 号注册表。
// 本档是「待开发什么」的唯一查询单点（替代全库 📋 语义扫描——首次盘点实测 15 分钟/4M tokens）。
func TestReqIndexOpenSet(t *testing.T) {
	idx := readRepo(t, "docs/19_REQ状态索引.md")
	reg := readRepo(t, "docs/18_REQ编号注册表.md")
	valid := map[string]bool{"📋": true, "🚧": true, "🔁": true, "🧊": true}
	reReq := regexp.MustCompile(`REQ-\d+`)
	rows := 0
	for _, line := range strings.Split(idx, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) < 3 {
			continue
		}
		st := strings.TrimSpace(cols[2])
		if !valid[st] {
			continue // 表头/分隔行/非状态行
		}
		rows++
		if strings.Contains(line, "✅") || strings.Contains(line, "⛔") {
			t.Errorf("19 号索引禁止完结态行（✅/⛔=完结即删行，历史在 18 号）：%.50s", line)
		}
		for _, req := range reReq.FindAllString(line, -1) {
			if !strings.Contains(reg, req) {
				t.Errorf("19 号索引引用的 %s 在 18 号注册表中不存在", req)
			}
		}
	}
	if rows < 40 {
		t.Errorf("19 号开放需求行数 %d 异常偏少（建档初版 56 行）——疑似误删", rows)
	}
}
