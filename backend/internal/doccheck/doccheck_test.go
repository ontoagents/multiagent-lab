// Package doccheck REQ-399/M195：docs 大档一致性校验器（常跑测试承载——沿 knowledge 校验器先例）。
// 三件核验（03 §2.59 详设口径）：
//
//	①交叉引用存在性——docs 大档引用的 REQ-nnn 必须在 18 号注册表主表在册；
//	②头部版本与迭代表一致性——01/02/03/18/20 五档头部状态行版本号必须在各自迭代表在行
//	 （防「头部 bump 迭代表未落」的滚动漂移——03 头部 rebase 回退史/02 M117 滞后同类实证）；
//	③18 注册表引用核对——18 迭代表各版本行号单调（同版本号重复出现=撞号，允许「撞号注」明示豁免）。
//
// 覆盖档=01/02/03/18/20 + handbook/04_本体/92 号（原 docs/04，2026-11-01 四区收敛 REQ-570
// 批次 1.5 迁入，无迭代表仅参与①）；docs/12 保留为①的 known 集合源（账本壳仍在 docs/ 根）。
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
	"docs/handbook/04_本体/92_本体_方案设计_架构现状.md", // 原 docs/04_本体_方案设计.md（2026-11-01 迁入）
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
		"docs/18A_REQ归档册.md", // REQ-499 拆册：完结注册行归档册与 18 号同为位号事实源
		"docs/01_智能体_需求文档_PRD.md",
		"docs/03_本体_需求文档.md",
		"docs/11_知识库_需求文档.md", // KB 域 REQ 行（REQ-102/241 等在册处）
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
	reg := readRepo(t, "docs/18_REQ编号注册表.md") + readRepo(t, "docs/18A_REQ归档册.md") // REQ-499 拆册两册同权
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

// handbookDirs docs/handbook 七个子区（REQ-570 四区收敛，2026-11-01）。
var handbookDirs = []string{
	"01_整体设计", "02_智能体", "03_项目", "04_本体", "05_知识库", "06_技能", "07_设置",
}

// handbookCanonicalDocs 24 档现状档（清单权威=113 号 §5.1；批次 1 frontmatter 四字段基线）。
var handbookCanonicalDocs = []string{
	"01_整体设计/00_整体设计.md",
	"01_整体设计/16_部署与运行.md",
	"01_整体设计/17_产品_信息架构与界面设计.md",
	"01_整体设计/31_平台知识页面_实现状态.md",
	"01_整体设计/90_整体架构与开源演进路线_20261029.md",
	"01_整体设计/110_产品_定位与规划.md",
	"02_智能体/00_智能体模块.md",
	"02_智能体/53_Harness_设计实现与演进.md",
	"02_智能体/54_Context_设计实现与演进.md",
	"02_智能体/55_Loop_设计实现与演进.md",
	"02_智能体/56_Graph_设计实现与演进.md",
	"03_项目/00_项目模块.md",
	"03_项目/01_工作区.md",
	"04_本体/00_本体模块.md",
	"04_本体/01_学习中心.md",
	"04_本体/02_构建.md",
	"04_本体/03_资产.md",
	"04_本体/04_运行.md",
	"04_本体/05_消费与审计.md",
	"04_本体/90_本体_底层实现原理.md",
	"04_本体/91_本体_专业概念手册.md",
	"05_知识库/00_知识库模块.md",
	"06_技能/00_技能模块.md",
	"07_设置/00_设置模块.md",
}

// TestHandbookStructure ⑦（2026-11-01 四区收敛 REQ-570 批次 1.5）：docs/handbook 结构项——
// **只查结构、不查内容一致性**（113 号 §3.7 口径收窄拍板）：①7 个子区目录与其 00 导读在位；
// ②24 档现状档 frontmatter 四字段 module/topic/desc/synced 非空且 synced 为 YYYY-MM-DD
// （README 豁免——handbook/README.md 不带 frontmatter）；③全档数下限 25（24 现状档+README，
// 92 号/01_知识库 等新增档允许增长、不允许塌缩）。
func TestHandbookStructure(t *testing.T) {
	// ①七子区与 00 导读在位
	for _, d := range handbookDirs {
		entries, err := os.ReadDir(filepath.Join(repoRoot, "docs/handbook", d))
		if err != nil {
			t.Fatalf("handbook 子区目录缺失：%s: %v", d, err)
		}
		found := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "00_") && strings.HasSuffix(e.Name(), ".md") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("handbook 子区 %s 缺 00 导读档", d)
		}
	}
	// ②24 档 frontmatter 四字段非空+synced 日期格式（README 豁免）
	syncedRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	for _, rel := range handbookCanonicalDocs {
		fields := parseFrontmatter(readRepo(t, "docs/handbook/"+rel))
		for _, k := range []string{"module", "topic", "desc", "synced"} {
			if fields[k] == "" {
				t.Errorf("docs/handbook/%s frontmatter 缺 %s 字段", rel, k)
			}
		}
		if !syncedRe.MatchString(fields["synced"]) {
			t.Errorf("docs/handbook/%s synced 应为 YYYY-MM-DD：%q", rel, fields["synced"])
		}
	}
	// ③全档数下限：README + 24 现状档（子区一层扁平结构，新增档只增不减）
	readme, err := os.Stat(filepath.Join(repoRoot, "docs/handbook", "README.md"))
	if err != nil || readme.IsDir() {
		t.Errorf("docs/handbook/README.md 缺失（豁免 frontmatter 但档案必须在位）")
	}
	count := 1 // README.md
	for _, d := range handbookDirs {
		entries, err := os.ReadDir(filepath.Join(repoRoot, "docs/handbook", d))
		if err != nil {
			continue // ① 已 Fatal
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				count++
			}
		}
	}
	if count < 25 {
		t.Errorf("docs/handbook 档数 %d 低于下限 25（24 现状档+README）——疑似现状档误删", count)
	}
}

// parseFrontmatter 解析 md 文件头部 `---` 包围的 key: value 块（缺块返回空 map）。
func parseFrontmatter(text string) map[string]string {
	fields := map[string]string{}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fields
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		i := strings.Index(line, ":")
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		fields[k] = strings.Trim(v, `"`)
	}
	return fields
}
