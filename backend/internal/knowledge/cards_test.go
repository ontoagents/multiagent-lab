package knowledge

// REQ-372/M170 知识卡校验器测试（无 build tag，常跑——知识卡是产品资产，漂移即缺陷；
// 与 evaldata TestCQDatasetConsistency 同纪律）。

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const ksRoot = "../../../platform-knowledge/09_知识体系"

// cardFrontmatter 卡 frontmatter 的极简解析结构（行式 YAML 子集：key: value + 两空格缩进列表项）。
type cardFrontmatter struct {
	fields map[string]string
	lists  map[string][]string
	body   string
}

func parseFrontmatter(t *testing.T, path string) cardFrontmatter {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败 %s: %v", path, err)
	}
	text := string(raw)
	fm := cardFrontmatter{fields: map[string]string{}, lists: map[string][]string{}}
	if !strings.HasPrefix(text, "---\n") {
		t.Fatalf("%s 缺 frontmatter（须以 --- 开头）", path)
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		t.Fatalf("%s frontmatter 未闭合", path)
	}
	fm.body = strings.TrimSpace(rest[end+4:])
	curKey := ""
	for _, line := range strings.Split(rest[:end], "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(line, "  - ") || strings.HasPrefix(line, "- ") {
			item := strings.TrimPrefix(strings.TrimPrefix(line, "  - "), "- ")
			item = strings.Trim(item, `[]"`)
			if curKey != "" && item != "" {
				fm.lists[curKey] = append(fm.lists[curKey], item)
			}
			continue
		}
		kv := strings.SplitN(trimmed, ":", 2)
		if len(kv) != 2 {
			continue
		}
		curKey = strings.TrimSpace(kv[0])
		raw := strings.TrimSpace(kv[1])
		fm.fields[curKey] = strings.Trim(raw, `[]"`)
		// 单行内联数组（key: ["a", "b"]）→ lists（与 graph.go ParseCardText 同口径修复：
		// 现网卡 prereq 实为内联数组形态，此前仅进 fields 致 ⑤ 断链检测对全部真实卡空转）
		if strings.HasPrefix(raw, "[") {
			if i := strings.LastIndex(raw, "]"); i > 0 {
				for _, it := range strings.Split(raw[1:i], ",") {
					it = strings.Trim(strings.TrimSpace(it), `"'`)
					if it != "" {
						fm.lists[curKey] = append(fm.lists[curKey], it)
					}
				}
			}
		}
	}
	return fm
}

var syncedDateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func TestKnowledgeCardsSchema(t *testing.T) {
	if _, err := os.Stat(ksRoot); err != nil {
		t.Skipf("知识体系目录不可达（%v）——仓库形态随迁", err)
	}
	// 多线遍历（REQ-372 P3 收口：01_本体线/02_智能体线/03_结合线）——每条线=含 cards/ 子目录的
	// 子目录；各线下限沿 58 号 §九 路线（本体线 12~15/智能体线 10~12/结合线 3~5）。
	entries, err := os.ReadDir(ksRoot)
	if err != nil {
		t.Fatalf("知识体系目录不可读: %v", err)
	}
	var lineDirs []string
	for _, e := range entries {
		if e.IsDir() {
			if _, cerr := os.Stat(filepath.Join(ksRoot, e.Name(), "cards")); cerr == nil {
				lineDirs = append(lineDirs, e.Name())
			}
		}
	}
	if len(lineDirs) < 3 {
		t.Fatalf("应至少三条学习线（本体/智能体/结合），实际 %d: %v", len(lineDirs), lineDirs)
	}
	sort.Strings(lineDirs)
	minCards := map[string]int{"01_本体线": 12, "02_智能体线": 10, "03_结合线": 3}
	registryText := readRepoFile(t, "../../../docs/18_REQ编号注册表.md") + readRepoFile(t, "../../../docs/03_本体_需求文档.md") + readRepoFile(t, "../../../docs/01_智能体_需求文档_PRD.md")

	for _, line := range lineDirs {
		lineDir := filepath.Join(ksRoot, line)
		cardsDir := filepath.Join(lineDir, "cards")
		cardFiles, err := os.ReadDir(cardsDir)
		if err != nil {
			t.Fatalf("cards 目录缺失: %v", err)
		}
		cardCount := 0
		for _, e := range cardFiles {
			validateCard(t, line, cardsDir, e, registryText)
			cardCount++
		}
		if lo := minCards[line]; lo > 0 && cardCount < lo {
			t.Errorf("%s 应 ≥%d 卡（58 号 §九），实际 %d", line, lo, cardCount)
		}
		// 主档索引完备：学习路径主档必须索引全部卡
		mainText := readRepoFile(t, mainDocPath(t, lineDir))
		for _, e := range cardFiles {
			if !strings.Contains(mainText, e.Name()) {
				t.Errorf("%s 主档未索引卡 %s", line, e.Name())
			}
		}
	}
}

// mainDocPath 线目录下的主档（00_*学习路径.md，唯一）。
func mainDocPath(t *testing.T, lineDir string) string {
	t.Helper()
	ms, err := filepath.Glob(filepath.Join(lineDir, "00_*学习路径.md"))
	if err != nil || len(ms) != 1 {
		t.Fatalf("%s 缺唯一主档 00_*学习路径.md（glob=%d）", lineDir, len(ms))
	}
	return ms[0]
}

// validateCard 单卡校验（①必备字段与枚举 ②篇幅 ③sources 存在性 ④check 1~4 ⑤prereq 断链）。
func validateCard(t *testing.T, line, cardsDir string, e os.DirEntry, registryText string) {
	t.Helper()
	if !strings.HasSuffix(e.Name(), ".md") {
		t.Fatalf("cards/ 内只允许 .md 文件，发现 %s", e.Name())
	}
	lengthRange := map[string][2]int{
		"concept":  {300, 800},
		"decision": {400, 1000},
		"practice": {400, 800},
	}
	path := filepath.Join(cardsDir, e.Name())
	fm := parseFrontmatter(t, path)
	rel := "platform-knowledge/09_知识体系/" + line + "/cards/" + e.Name()

	// ① 必备字段与枚举
	for _, k := range []string{"module", "topic", "desc", "synced", "kind", "domain", "sources", "check"} {
		if _, ok := fm.fields[k]; !ok {
			if _, isList := fm.lists[k]; !isList {
				t.Errorf("%s 缺必备字段 %s", rel, k)
			}
		}
	}
	if fm.fields["module"] != "知识体系" {
		t.Errorf("%s module 应为 知识体系，实际 %q", rel, fm.fields["module"])
	}
	if !syncedDateRE.MatchString(fm.fields["synced"]) {
		t.Errorf("%s synced 应为 YYYY-MM-DD，实际 %q", rel, fm.fields["synced"])
	}
	kind := fm.fields["kind"]
	if _, ok := lengthRange[kind]; !ok {
		t.Errorf("%s kind 非法（concept|decision|practice）: %q", rel, kind)
		return
	}
	switch fm.fields["domain"] {
	case "ontology", "agent", "kb", "bridge":
	default:
		t.Errorf("%s domain 非法: %q", rel, fm.fields["domain"])
	}

	// ② 篇幅（正文非空白 rune 数）
	bodyRunes := len([]rune(strings.Join(strings.Fields(fm.body), "")))
	lo, hi := lengthRange[kind][0], lengthRange[kind][1]
	if bodyRunes < lo || bodyRunes > hi {
		t.Errorf("%s 正文 %d 字超出 %s 型篇幅 [%d,%d]", rel, bodyRunes, kind, lo, hi)
	}

	// ③ sources 源指针存在性（硬纪律）
	srcs := fm.lists["sources"]
	if len(srcs) == 0 {
		t.Errorf("%s 缺 sources（硬纪律：每卡必带源指针）", rel)
	}
	for _, s := range srcs {
		validateSource(t, rel, s, registryText)
	}

	// ④ check 自检问题 1~4 个
	if n := len(fm.lists["check"]); n < 1 || n > 4 {
		t.Errorf("%s check 应 1~4 个，实际 %d", rel, n)
	}

	// ⑤ prereq 断链检测（相对本卡目录解析；跨线 ../ 形态同口径）
	for _, pr := range fm.lists["prereq"] {
		pr = strings.TrimSpace(pr)
		if pr == "" {
			continue
		}
		target := filepath.Clean(filepath.Join(cardsDir, pr))
		if _, err := os.Stat(target); err != nil {
			t.Errorf("%s prereq 断链: %q（解析为 %s）", rel, pr, target)
		}
	}
}

// validateSource 单条源指针校验：文件路径形态须存在；REQ-nnn 须在 18 号出现；D-O* 跳过。
func validateSource(t *testing.T, card, source, registryText string) {
	t.Helper()
	s := strings.TrimSpace(source)
	if s == "" {
		t.Errorf("%s 存在空 source 条目", card)
		return
	}
	// 文件路径形态（可带 § 定位后缀）
	if i := strings.Index(s, " §"); i > 0 {
		s = s[:i]
	}
	if strings.HasSuffix(s, ".md") || strings.Contains(s, "/") {
		// 仓库相对路径锚定仓库根（测试 cwd 为本包目录，三级向上）
		if _, err := os.Stat("../../../" + s); err != nil {
			t.Errorf("%s source 文件不存在: %q", card, s)
		}
		return
	}
	// REQ-nnn 形态
	if re := regexp.MustCompile(`^REQ-\d+`); re.MatchString(s) {
		if !strings.Contains(registryText, regexp.MustCompile(`^REQ-\d+`).FindString(s)) {
			t.Errorf("%s source REQ 引用未在 18 号注册表出现: %q", card, s)
		}
		return
	}
	// D-O* / 01~99 号档引用 / 纯文字锚：跳过（语义正确性留人工抽检）
}

func TestKnowledgeHomeAndPathDocs(t *testing.T) {
	// 导读：role: home 必备（页面组锚点）
	home := parseFrontmatter(t, filepath.Join(ksRoot, "00_导读.md"))
	if home.fields["role"] != "home" {
		t.Fatal("00_导读.md 缺 role: home")
	}
	if home.fields["module"] != "知识体系" {
		t.Fatalf("导读 module 应为 知识体系: %q", home.fields["module"])
	}
	// 主档：单源纪律——正文不含成段复制（启发式：各线主档均 <6000 rune）
	lineDirs, err := os.ReadDir(ksRoot)
	if err != nil {
		t.Fatalf("知识体系目录不可读: %v", err)
	}
	for _, e := range lineDirs {
		if !e.IsDir() {
			continue
		}
		ms, _ := filepath.Glob(filepath.Join(ksRoot, e.Name(), "00_*学习路径.md"))
		if len(ms) != 1 {
			continue // 非学习线目录（无主档）跳过
		}
		mainText := readRepoFile(t, ms[0])
		if n := len([]rune(mainText)); n > 6000 {
			t.Errorf("%s 主档应保持路标形态（<6000 rune），实际 %d——内容疑似复制进主档（破单源）", ms[0], n)
		}
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", rel, err)
	}
	return string(b)
}
